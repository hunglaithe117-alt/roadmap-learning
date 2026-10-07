package sync

import (
	"context"
	"fmt"
	"sort"
	"time"

	srsdomain "langapp/internal/domain/srs"
	domain "langapp/internal/domain/sync"
)

// SkipTables lists tables intentionally skipped during merge.
var SkipTables = []string{"dict", "en_dict", "schema_migrations", "goose_db_version"}

// Merge merges a peer snapshot into the local database in a single transaction.
func (s *Service) Merge(ctx context.Context, snap domain.PeerSnapshot) (MergeResult, error) {
	now := s.timestampNow()

	localV, err := s.repo.SchemaVersion(ctx)
	if err != nil {
		return MergeResult{}, fmt.Errorf("đọc schema version local: %w", err)
	}
	peerV, err := s.repo.PeerSchemaVersion(ctx)
	if err != nil {
		return MergeResult{}, fmt.Errorf("đọc schema version peer: %w", err)
	}
	if err := domain.CheckVersion(localV, peerV); err != nil {
		return MergeResult{}, newError(StatusBadRequest, "%s", err.Error())
	}
	lastSync, err := s.repo.LastSyncAt(ctx)
	if err != nil {
		return MergeResult{}, fmt.Errorf("đọc last_sync_at: %w", err)
	}

	var (
		merged Merged
		sink   = &conflictSink{repo: s.repo}
	)
	err = s.uow.Do(ctx, func(tx Tx) error {
		sink.tx = tx
		if err := s.mergeDecks(ctx, tx, snap, lastSync, now, sink, &merged); err != nil {
			return err
		}
		adopted, err := s.mergeCards(ctx, tx, snap, lastSync, now, sink, &merged)
		if err != nil {
			return err
		}
		if err := s.mergeRoadmap(ctx, tx, snap, lastSync, now, sink, &merged); err != nil {
			return err
		}
		if err := s.mergeReviews(ctx, tx, snap, sink, now, &merged, adopted); err != nil {
			return err
		}
		if err := s.mergeNotes(ctx, tx, snap, sink, &merged, adopted); err != nil {
			return err
		}
		return s.repo.SetLastSyncAt(ctx, tx, now)
	})
	if err != nil {
		return MergeResult{}, err
	}
	warnings := domain.DetectClockSkew(snap.MaxUpdatedAt, now)
	return MergeResult{
		OK: true, Merged: merged, Conflicts: sink.all, Warnings: warnings, LastSyncAt: now,
	}, nil
}

// localRowPtr returns a pointer to local row, or nil if not found.
func localRowPtr(r domain.Row) *domain.Row { return &r }

func row(table domain.Table, guid, updated, created string, deleted int, values map[string]string) domain.Row {
	return domain.Row{
		Table: table, GUID: guid, UpdatedAt: updated, CreatedAt: created,
		Deleted: deleted, Values: values,
	}
}

// mergeDecks applies LWW merge to decks table.
func (s *Service) mergeDecks(ctx context.Context, tx Tx, snap domain.PeerSnapshot,
	lastSync, now string, sink *conflictSink, merged *Merged) error {
	local, err := s.repo.DeckRows(ctx, tx)
	if err != nil {
		return fmt.Errorf("đọc deck local: %w", err)
	}
	for _, in := range deckRowsFrom(snap) {
		cur, found := local[in.GUID]
		inRow := row(domain.TableDecks, in.GUID, in.UpdatedAt, in.CreatedAt, in.Deleted,
			deckValues(in))
		var localRow *domain.Row
		if found {
			localRow = localRowPtr(row(domain.TableDecks, cur.GUID, cur.UpdatedAt,
				cur.CreatedAt, cur.Deleted, deckValues(cur)))
		}
		d := domain.Decide(domain.Merge{Local: localRow, Incoming: inRow,
			LastSync: lastSync, Now: now})
		switch d.Action {
		case domain.ActionInsert:
			id, err := s.repo.UpsertDeck(ctx, tx, in, false)
			if err != nil {
				return fmt.Errorf("insert deck %s: %w", in.GUID, err)
			}
			in.ID = id
			local[in.GUID] = in
			merged.Decks++
		case domain.ActionUpdate:
			in.ID = cur.ID
			if _, err := s.repo.UpsertDeck(ctx, tx, in, true); err != nil {
				return fmt.Errorf("update deck %s: %w", in.GUID, err)
			}
			local[in.GUID] = in
			merged.Decks++
		}
		sink.add(ctx, d.Conflict)
	}
	return nil
}

func deckValues(d DeckRow) map[string]string {
	return map[string]string{"name": d.Name, "lang": d.Lang}
}

// mergeCards applies LWW merge to cards table and remaps deck_id via GUID.
func (s *Service) mergeCards(ctx context.Context, tx Tx, snap domain.PeerSnapshot,
	lastSync, now string, sink *conflictSink, merged *Merged) (map[string]int64, error) {
	local, err := s.repo.CardRows(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("đọc card local: %w", err)
	}
	deckIDByGUID, err := s.deckIDs(ctx, tx)
	if err != nil {
		return nil, err
	}
	adopted := map[string]int64{}
	for _, in := range cardRowsFrom(snap) {
		deckID, ok := deckIDByGUID[in.DeckGUID]
		if !ok {
			continue
		}
		in.DeckID = &deckID
		cur, found := local[in.GUID]
		inRow := row(domain.TableCards, in.GUID, in.UpdatedAt, in.CreatedAt, in.Deleted,
			cardValues(in))
		var localRow *domain.Row
		if found {
			localRow = localRowPtr(row(domain.TableCards, cur.GUID, cur.UpdatedAt, cur.CreatedAt,
				cur.Deleted, cardValues(cur)))
		}
		d := domain.Decide(domain.Merge{Local: localRow, Incoming: inRow,
			LastSync: lastSync, Now: now})

		switch d.Action {
		case domain.ActionUpdate:
			in.ID = cur.ID
			if _, _, err := s.repo.UpsertCard(ctx, tx, in, true); err != nil {
				return nil, fmt.Errorf("update card %s: %w", in.GUID, err)
			}
			local[in.GUID] = in
			merged.Cards++
		case domain.ActionInsert:
			tombID, tombGUID, ok, err := s.repo.TombstoneCard(ctx, tx, deckID, in.Front)
			if err != nil {
				return nil, fmt.Errorf("tra tombstone card: %w", err)
			}
			if ok {
				if in.Deleted == 0 {
					in.ID = tombID
					if _, _, err := s.repo.UpsertCard(ctx, tx, in, true); err != nil {
						return nil, fmt.Errorf("hồi sinh card %s: %w", in.GUID, err)
					}
					delete(local, tombGUID)
					local[in.GUID] = in
					adopted[tombGUID] = tombID
					merged.Cards++
					sink.add(ctx, &domain.Conflict{
						Table: domain.TableCards, GUID: in.GUID, Winner: domain.WinnerIncoming,
						Detail: fmt.Sprintf("recreate-adopted-tombstone local_guid=%s front=%q id=%d",
							tombGUID, in.Front, tombID),
						At: now,
					})
				}
				continue
			}
			if liveID, liveGUID, found, err := s.repo.LiveCardGUIDByFront(ctx, tx, deckID, in.Front); err != nil {
				return nil, fmt.Errorf("tra thẻ sống cùng front %q: %w", in.Front, err)
			} else if found {
				if _, tracked := local[liveGUID]; tracked {
					adopted[in.GUID] = liveID
					sink.add(ctx, &domain.Conflict{
						Table: domain.TableCards, GUID: in.GUID, Winner: domain.WinnerLocal,
						Detail: fmt.Sprintf("peer-row-collapses-into-live-card front=%q kept_guid=%s",
							in.Front, liveGUID),
						At: now,
					})
					continue
				}
			}
			id, dup, err := s.repo.UpsertCard(ctx, tx, in, false)
			if err != nil {
				return nil, fmt.Errorf("insert card %s: %w", in.GUID, err)
			}
			if dup {
				sink.add(ctx, &domain.Conflict{
					Table: domain.TableCards, GUID: in.GUID, Winner: domain.WinnerLocal,
					Detail: fmt.Sprintf("unique-live-duplicate kept local front=%q", in.Front),
					At:     now,
				})
			}
			in.ID = id
			local[in.GUID] = in
			merged.Cards++
		}
		sink.add(ctx, d.Conflict)
	}
	return adopted, nil
}

func cardValues(c CardRow) map[string]string {
	out := map[string]string{
		"front": c.Front, "back": c.Back, "pinyin": c.Pinyin,
		"due_at": c.DueAt, "state": c.State,
	}
	for k, v := range map[string]*string{
		"tone": c.Tone, "ipa": c.IPA, "stress": c.Stress, "audio_url": c.AudioURL,
	} {
		if v != nil {
			out[k] = *v
		}
	}
	return out
}

func (s *Service) deckIDs(ctx context.Context, tx Tx) (map[string]int64, error) {
	rows, err := s.repo.DeckRows(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("đọc lại deck local: %w", err)
	}
	out := make(map[string]int64, len(rows))
	for guid, r := range rows {
		if guid != "" {
			out[guid] = r.ID
		}
	}
	return out, nil
}

func (s *Service) cardIDs(ctx context.Context, tx Tx) (map[string]int64, error) {
	rows, err := s.repo.CardRows(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("đọc lại card local: %w", err)
	}
	out := make(map[string]int64, len(rows))
	for guid, r := range rows {
		if guid != "" {
			out[guid] = r.ID
		}
	}
	return out, nil
}

// mergeRoadmap applies LWW merge across all roadmap tables and bookmarks.
func (s *Service) mergeRoadmap(ctx context.Context, tx Tx, snap domain.PeerSnapshot,
	lastSync, now string, sink *conflictSink, merged *Merged) error {
	pathIDByGUID, err := s.mergePaths(ctx, tx, snap, lastSync, now, sink, merged)
	if err != nil {
		return err
	}
	stageIDByGUID, err := s.mergeStages(ctx, tx, snap, lastSync, now, sink, merged, pathIDByGUID)
	if err != nil {
		return err
	}
	if err := s.mergeMilestones(ctx, tx, snap, lastSync, now, sink, merged, stageIDByGUID); err != nil {
		return err
	}
	topicIDByGUID, err := s.mergeTopics(ctx, tx, snap, lastSync, now, sink, merged, stageIDByGUID)
	if err != nil {
		return err
	}
	if err := s.mergeResources(ctx, tx, snap, lastSync, now, sink, merged, topicIDByGUID); err != nil {
		return err
	}
	return s.mergeBookmarks(ctx, tx, snap, lastSync, now, sink, merged)
}

func (s *Service) mergePaths(ctx context.Context, tx Tx, snap domain.PeerSnapshot,
	lastSync, now string, sink *conflictSink, merged *Merged) (map[string]int64, error) {
	local, err := s.repo.PathRows(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("đọc roadmap_paths local: %w", err)
	}
	ids := make(map[string]int64, len(local))
	for g, r := range local {
		ids[g] = r.ID
	}
	for _, in := range pathRowsFrom(snap) {
		cur, found := local[in.GUID]
		inRow := row(domain.TableRoadmapPaths, in.GUID, in.UpdatedAt, in.CreatedAt, in.Deleted,
			map[string]string{"slug": in.Slug, "title": in.Title})
		var localRow *domain.Row
		if found {
			localRow = localRowPtr(row(domain.TableRoadmapPaths, cur.GUID, cur.UpdatedAt, cur.CreatedAt,
				cur.Deleted, map[string]string{"slug": cur.Slug, "title": cur.Title}))
		}
		d := domain.Decide(domain.Merge{Local: localRow, Incoming: inRow,
			LastSync: lastSync, Now: now})
		switch d.Action {
		case domain.ActionInsert, domain.ActionUpdate:
			in.ID = cur.ID
			newID, skipped, err := s.repo.UpsertPath(ctx, tx, in, d.Action == domain.ActionUpdate)
			if err != nil {
				return nil, fmt.Errorf("ghi roadmap_paths %s: %w", in.GUID, err)
			}
			if skipped {
				sink.addSkipped(ctx, domain.TableRoadmapPaths, in.GUID, in.Slug, now)
				continue
			}
			ids[in.GUID] = newID
			merged.RoadmapPaths++
		}
		sink.add(ctx, d.Conflict)
	}
	return ids, nil
}

func (s *Service) mergeStages(ctx context.Context, tx Tx, snap domain.PeerSnapshot,
	lastSync, now string, sink *conflictSink, merged *Merged, pathIDByGUID map[string]int64) (map[string]int64, error) {
	local, err := s.repo.StageRows(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("đọc roadmap_stages local: %w", err)
	}
	ids := make(map[string]int64, len(local))
	for g, r := range local {
		ids[g] = r.ID
	}
	for _, in := range stageRowsFrom(snap) {
		if _, ok := pathIDByGUID[in.PathGUID]; !ok {
			continue
		}
		cur, found := local[in.GUID]
		localDeck := ""
		if found && cur.DeckGUID != nil {
			localDeck = *cur.DeckGUID
		}
		inRow := row(domain.TableRoadmapStages, in.GUID, in.UpdatedAt, in.CreatedAt, in.Deleted,
			stageValues(in, localDeck))
		var localRow *domain.Row
		if found {
			localRow = localRowPtr(row(domain.TableRoadmapStages, cur.GUID, cur.UpdatedAt, cur.CreatedAt,
				cur.Deleted, stageValues(cur, localDeck)))
		}
		d := domain.Decide(domain.Merge{Local: localRow, Incoming: inRow,
			LastSync: lastSync, Now: now})
		switch d.Action {
		case domain.ActionInsert, domain.ActionUpdate:
			in.ID = cur.ID
			newID, skipped, err := s.repo.UpsertStage(ctx, tx, in, d.Action == domain.ActionUpdate)
			if err != nil {
				return nil, fmt.Errorf("ghi roadmap_stages %s: %w", in.GUID, err)
			}
			if skipped {
				sink.addSkipped(ctx, domain.TableRoadmapStages, in.GUID, in.Slug, now)
				continue
			}
			ids[in.GUID] = newID
			merged.RoadmapStages++
		}
		sink.add(ctx, d.Conflict)
	}
	return ids, nil
}

// stageValues builds comparison map for stage LWW decisions.
func stageValues(st StageRow, localDeck string) map[string]string {
	out := map[string]string{
		"slug": st.Slug, "title": st.Title, "status": st.Status,
		"terrain": st.Terrain, "direction": st.Direction,
		"deck_guid": localDeck,
	}
	if st.DeckGUID != nil {
		out["deck_guid"] = *st.DeckGUID
	}
	if st.CompletedAt != nil {
		out["completed_at"] = *st.CompletedAt
	}
	return out
}

func (s *Service) mergeMilestones(ctx context.Context, tx Tx, snap domain.PeerSnapshot,
	lastSync, now string, sink *conflictSink, merged *Merged, stageIDByGUID map[string]int64) error {
	local, err := s.repo.MilestoneRows(ctx, tx)
	if err != nil {
		return fmt.Errorf("đọc roadmap_milestones local: %w", err)
	}
	for _, in := range milestoneRowsFrom(snap) {
		if _, ok := stageIDByGUID[in.StageGUID]; !ok {
			continue
		}
		cur, found := local[in.GUID]
		inRow := row(domain.TableRoadmapMilestones, in.GUID, in.UpdatedAt, in.CreatedAt, in.Deleted,
			map[string]string{"text": in.Text})
		var localRow *domain.Row
		if found {
			localRow = localRowPtr(row(domain.TableRoadmapMilestones, cur.GUID, cur.UpdatedAt, cur.CreatedAt,
				cur.Deleted, map[string]string{"text": cur.Text}))
		}
		d := domain.Decide(domain.Merge{Local: localRow, Incoming: inRow,
			LastSync: lastSync, Now: now})
		switch d.Action {
		case domain.ActionInsert, domain.ActionUpdate:
			in.ID = cur.ID
			newID, skipped, err := s.repo.UpsertMilestone(ctx, tx, in, d.Action == domain.ActionUpdate)
			if err != nil {
				return fmt.Errorf("ghi roadmap_milestones %s: %w", in.GUID, err)
			}
			if skipped {
				sink.addSkipped(ctx, domain.TableRoadmapMilestones, in.GUID, in.Text, now)
				continue
			}
			in.ID = newID
			local[in.GUID] = in
			merged.RoadmapMilestones++
		}
		sink.add(ctx, d.Conflict)
	}
	return nil
}

func (s *Service) mergeTopics(ctx context.Context, tx Tx, snap domain.PeerSnapshot,
	lastSync, now string, sink *conflictSink, merged *Merged, stageIDByGUID map[string]int64) (map[string]int64, error) {
	local, err := s.repo.TopicRows(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("đọc roadmap_topics local: %w", err)
	}
	ids := make(map[string]int64, len(local))
	for g, r := range local {
		ids[g] = r.ID
	}
	for _, in := range topicRowsFrom(snap) {
		if _, ok := stageIDByGUID[in.StageGUID]; !ok {
			continue
		}
		cur, found := local[in.GUID]
		inRow := row(domain.TableRoadmapTopics, in.GUID, in.UpdatedAt, in.CreatedAt, in.Deleted,
			topicValues(in))
		var localRow *domain.Row
		if found {
			localRow = localRowPtr(row(domain.TableRoadmapTopics, cur.GUID, cur.UpdatedAt, cur.CreatedAt,
				cur.Deleted, topicValues(cur)))
		}
		d := domain.Decide(domain.Merge{Local: localRow, Incoming: inRow,
			LastSync: lastSync, Now: now})
		switch d.Action {
		case domain.ActionInsert, domain.ActionUpdate:
			in.ID = cur.ID
			newID, skipped, err := s.repo.UpsertTopic(ctx, tx, in, d.Action == domain.ActionUpdate)
			if err != nil {
				return nil, fmt.Errorf("ghi roadmap_topics %s: %w", in.GUID, err)
			}
			if skipped {
				sink.addSkipped(ctx, domain.TableRoadmapTopics, in.GUID, in.Title, now)
				continue
			}
			ids[in.GUID] = newID
			merged.RoadmapTopics++
		}
		sink.add(ctx, d.Conflict)
	}
	return ids, nil
}

func topicValues(tp TopicRow) map[string]string {
	out := map[string]string{
		"title": tp.Title, "status": tp.Status, "is_optional": itoa(tp.IsOptional),
	}
	if tp.CompletedAt != nil {
		out["completed_at"] = *tp.CompletedAt
	}
	if tp.MapX != nil {
		out["map_x"] = ftoa(*tp.MapX)
	}
	if tp.MapY != nil {
		out["map_y"] = ftoa(*tp.MapY)
	}
	return out
}

func (s *Service) mergeResources(ctx context.Context, tx Tx, snap domain.PeerSnapshot,
	lastSync, now string, sink *conflictSink, merged *Merged, topicIDByGUID map[string]int64) error {
	local, err := s.repo.ResourceRows(ctx, tx)
	if err != nil {
		return fmt.Errorf("đọc roadmap_resources local: %w", err)
	}
	for _, in := range resourceRowsFrom(snap) {
		if _, ok := topicIDByGUID[in.TopicGUID]; !ok {
			continue
		}
		cur, found := local[in.GUID]
		inRow := row(domain.TableRoadmapResources, in.GUID, in.UpdatedAt, in.CreatedAt, in.Deleted,
			resourceValues(in))
		var localRow *domain.Row
		if found {
			localRow = localRowPtr(row(domain.TableRoadmapResources, cur.GUID, cur.UpdatedAt, cur.CreatedAt,
				cur.Deleted, resourceValues(cur)))
		}
		d := domain.Decide(domain.Merge{Local: localRow, Incoming: inRow,
			LastSync: lastSync, Now: now})
		switch d.Action {
		case domain.ActionInsert, domain.ActionUpdate:
			in.ID = cur.ID
			newID, skipped, err := s.repo.UpsertResource(ctx, tx, in, d.Action == domain.ActionUpdate)
			if err != nil {
				return fmt.Errorf("ghi roadmap_resources %s: %w", in.GUID, err)
			}
			if skipped {
				sink.addSkipped(ctx, domain.TableRoadmapResources, in.GUID, in.Title, now)
				continue
			}
			in.ID = newID
			local[in.GUID] = in
			merged.RoadmapResources++
		}
		sink.add(ctx, d.Conflict)
	}
	return nil
}

func resourceValues(res ResourceRow) map[string]string {
	out := map[string]string{"title": res.Title, "kind": res.Kind, "note": res.Note}
	if res.URL != nil {
		out["url"] = *res.URL
	}
	return out
}

func (s *Service) mergeBookmarks(ctx context.Context, tx Tx, snap domain.PeerSnapshot,
	lastSync, now string, sink *conflictSink, merged *Merged) error {
	local, err := s.repo.BookmarkRows(ctx, tx)
	if err != nil {
		return fmt.Errorf("đọc roadmap_bookmarks local: %w", err)
	}
	for _, in := range bookmarkRowsFrom(snap) {
		cur, found := local[in.GUID]
		inRow := row(domain.TableRoadmapBookmarks, in.GUID, in.UpdatedAt, in.CreatedAt, in.Deleted,
			bookmarkValues(in))
		var localRow *domain.Row
		if found {
			localRow = localRowPtr(row(domain.TableRoadmapBookmarks, cur.GUID, cur.UpdatedAt, cur.CreatedAt,
				cur.Deleted, bookmarkValues(cur)))
		}
		d := domain.Decide(domain.Merge{Local: localRow, Incoming: inRow,
			LastSync: lastSync, Now: now})
		switch d.Action {
		case domain.ActionInsert, domain.ActionUpdate:
			in.ID = cur.ID
			newID, skipped, err := s.repo.UpsertBookmark(ctx, tx, in, d.Action == domain.ActionUpdate)
			if err != nil {
				return fmt.Errorf("ghi roadmap_bookmarks %s: %w", in.GUID, err)
			}
			if skipped {
				sink.addSkipped(ctx, domain.TableRoadmapBookmarks, in.GUID, in.Title, now)
				continue
			}
			in.ID = newID
			local[in.GUID] = in
			merged.RoadmapBookmarks++
		}
		sink.add(ctx, d.Conflict)
	}
	return nil
}

func bookmarkValues(b BookmarkRow) map[string]string {
	out := map[string]string{"title": b.Title, "status": b.Status, "tags": b.Tags, "note": b.Note}
	if b.URL != nil {
		out["url"] = *b.URL
	}
	return out
}

// mergeReviews unions reviews by GUID and recalculates card review schedules.
func (s *Service) mergeReviews(ctx context.Context, tx Tx, snap domain.PeerSnapshot,
	sink *conflictSink, now string, merged *Merged, adopted map[string]int64) error {
	known, err := s.repo.ReviewGUIDs(ctx, tx)
	if err != nil {
		return fmt.Errorf("đọc guid review local: %w", err)
	}
	cardIDByGUID, err := s.cardIDs(ctx, tx)
	if err != nil {
		return err
	}
	affected := map[int64]string{}
	for _, in := range reviewRowsFrom(snap) {
		if in.GUID == "" || known[in.GUID] {
			continue
		}
		cardID, ok := cardIDByGUID[in.CardGUID]
		if !ok {
			cardID, ok = adopted[in.CardGUID]
		}
		if !ok {
			continue
		}
		rowToWrite := in
		rowToWrite.CardID = &cardID
		added, err := s.repo.AppendReview(ctx, tx, rowToWrite)
		if err != nil {
			return fmt.Errorf("nạp review %s: %w", in.GUID, err)
		}
		if !added {
			continue
		}
		known[in.GUID] = true
		affected[cardID] = in.CardGUID
		merged.Reviews++
	}
	ids := make([]int64, 0, len(affected))
	for id := range affected {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, cardID := range ids {
		if err := s.replayCard(ctx, tx, cardID, affected[cardID], now, sink); err != nil {
			return err
		}
	}
	return nil
}

// replayCard recalculates card review schedule from full review history.
func (s *Service) replayCard(ctx context.Context, tx Tx, cardID int64, cardGUID, now string,
	sink *conflictSink) error {
	revs, err := s.repo.ReviewsOfCard(ctx, tx, cardID)
	if err != nil {
		return fmt.Errorf("đọc lịch sử ôn của card %d: %w", cardID, err)
	}
	if len(revs) == 0 {
		return nil
	}
	parsed := make([]srsdomain.Review, 0, len(revs))
	for _, r := range revs {
		at, err := time.Parse(time.RFC3339, r.ReviewedAt)
		if err != nil {
			at, _ = time.Parse(time.RFC3339, now)
		}
		parsed = append(parsed, srsdomain.Review{
			Grade: srsdomain.Grade(r.Grade), ReviewedAt: at.UTC(),
		})
	}
	reps, lapses, stability, difficulty, due := srsdomain.Replay(parsed)
	if err := s.repo.ReplayCard(ctx, tx, cardID, ReplayResult{
		Reps: reps, Lapses: lapses, Stability: stability, Difficulty: difficulty,
		DueAt: due.Format(time.RFC3339),
	}, now); err != nil {
		return fmt.Errorf("replay lịch card %d: %w", cardID, err)
	}
	sink.add(ctx, &domain.Conflict{
		Table: domain.TableReviews, GUID: cardGUID, Winner: domain.WinnerLocal,
		Detail: fmt.Sprintf("reps-recomputed từ %d review", len(revs)), At: now,
	})
	return nil
}

// mergeNotes unions notes by GUID.
func (s *Service) mergeNotes(ctx context.Context, tx Tx, snap domain.PeerSnapshot,
	sink *conflictSink, merged *Merged, adopted map[string]int64) error {
	known, err := s.repo.NoteGUIDs(ctx, tx)
	if err != nil {
		return fmt.Errorf("đọc guid note local: %w", err)
	}
	cardIDByGUID, err := s.cardIDs(ctx, tx)
	if err != nil {
		return err
	}
	for _, in := range noteRowsFrom(snap) {
		if in.GUID == "" || known[in.GUID] {
			continue
		}
		rowToWrite := in
		if in.CardGUID != "" {
			id, ok := cardIDByGUID[in.CardGUID]
			if !ok {
				id, ok = adopted[in.CardGUID]
			}
			if ok {
				rowToWrite.CardID = &id
			} else {
				rowToWrite.CardID = nil
			}
		}
		added, err := s.repo.AppendNote(ctx, tx, rowToWrite)
		if err != nil {
			return fmt.Errorf("nạp note %s: %w", in.GUID, err)
		}
		if added {
			known[in.GUID] = true
			merged.Notes++
		}
	}
	return nil
}
