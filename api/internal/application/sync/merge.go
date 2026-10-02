package sync

import (
	"context"
	"fmt"
	"sort"
	"time"

	srsdomain "langapp/internal/domain/srs"
	domain "langapp/internal/domain/sync"
)

// SkipTables là các bảng CỐ Ý không merge. Liệt kê ở đây để tài liệu hoá lý
// do, không chỉ để tránh quên:
//
//   - dict / en_dict: từ điển là DỮ LIỆU TĨNH, cùng phiên bản 2 máy đã có
//     giống nhau. Ghi đè là mất nghĩa user sửa tay.
//   - schema_migrations: version của DB local, không bao giờ lấy từ peer —
//     lấy thì máy này tự nhận là đã upgrade và migration sẽ không chạy.
//   - goose_db_version: goose tự quản lý, cùng lý do.
var SkipTables = []string{"dict", "en_dict", "schema_migrations", "goose_db_version"}

// Merge gộp snapshot peer vào DB local trong 1 transaction.
//
// Thứ tự bắt buộc (FK): decks → cards; paths → stages → {milestones, topics}
// → resources; reviews/notes sau cards vì chúng trỏ `card_id`.
//
// Mọi quyết định LWW / tombstone / ghi log conflict đi qua `domain/sync.Decide`
// — tầng này KHÔNG tự suy luận luật, chỉ thi hành `MergeDecision.Action`.
func (s *Service) Merge(ctx context.Context, snap domain.PeerSnapshot) (MergeResult, error) {
	now := s.timestampNow()

	// Chặn merge giữa 2 máy lệch schema TRƯỚC khi mở transaction: lý do v1 đã
	// chọn là "xóa DB làm lại, không migrate, không fallback" — dữ liệu sai kiểu
	// hỏng còn tệ hơn không đồng bộ.
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

// localRowPtr trả con trỏ tới row local đã đọc, hoặc nil nếu chưa có — đúng
// dạng `Decide` mong (nil = "chưa có bản local" → insert).
func localRowPtr(r domain.Row) *domain.Row { return &r }

func row(table domain.Table, guid, updated, created string, deleted int, values map[string]string) domain.Row {
	return domain.Row{
		Table: table, GUID: guid, UpdatedAt: updated, CreatedAt: created,
		Deleted: deleted, Values: values,
	}
}

// mergeDecks áp LWW lên bảng `decks`.
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
		sink.add(d.Conflict)
	}
	return nil
}

func deckValues(d DeckRow) map[string]string {
	return map[string]string{"name": d.Name, "lang": d.Lang}
}

// mergeCards áp LWW lên `cards`, remap `deck_id` qua guid.
func (s *Service) mergeCards(ctx context.Context, tx Tx, snap domain.PeerSnapshot,
	lastSync, now string, sink *conflictSink, merged *Merged) (map[string]int64, error) {
	local, err := s.repo.CardRows(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("đọc card local: %w", err)
	}
	// Bản đồ guid→id đọc LẠI trong transaction sau khi merge decks: đây là chỗ
	// duy nhất nhìn thấy cả deck vừa insert. Giữ map trong RAM sẽ bỏ sót chúng.
	deckIDByGUID, err := s.deckIDs(ctx, tx)
	if err != nil {
		return nil, err
	}
	// `adopted` là guid CŨ (tombstone local) → id của thẻ đã hồi sinh.
	//
	// Nó giữ 2 vai trò, cùng 1 nguyên nhân:
	//
	//  1. Chặn incoming mang guid cũ tạo thêm row. Peer gửi CẢ thẻ sống
	//     `c-aaa` LẪN tombstone `c-zzz` cùng front (tìm hiểu 1 thẻ đã xoá rồi
	//     tạo lại ở máy khác). Sau khi hồi sinh, `c-zzz` không còn tồn tại
	//     local; nếu insert nó, `ux_cards_deck_front` là partial
	//     (`WHERE deleted = 0`) nên KHÔNG chặn ⇒ mọc thêm 1 row tombstone rác
	//     mà user không bao giờ tạo.
	//  2. Giữ alias cho `reviews`/`notes` của peer vẫn trỏ đúng thẻ: `cardIDs`
	//     đọc lại từ DB nên không thấy `c-zzz` nữa; thiếu alias thì lịch sử
	//     ôn gắn với guid cũ bị bỏ im lặng.
	adopted := map[string]int64{}
	for _, in := range cardRowsFrom(snap) {
		deckID, ok := deckIDByGUID[in.DeckGUID]
		if !ok {
			// Deck cha chưa map được → bỏ card. Cùng version thì deck luôn đi
			// kèm card, nên trường hợp này = snapshot hỏng; bỏ thay vì tạo
			// card mồ côi (FK sẽ chặn rồi rollback cả merge).
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
			// `ID` là id số LOCAL — application biết vì nó vừa đọc `CardRows`.
			// KHÔNG bao giờ lấy id từ snapshot: id 2 máy trùng nhau.
			in.ID = cur.ID
			if _, _, err := s.repo.UpsertCard(ctx, tx, in, true); err != nil {
				return nil, fmt.Errorf("update card %s: %w", in.GUID, err)
			}
			local[in.GUID] = in
			merged.Cards++
		case domain.ActionInsert:
			// Peer RE-CREATE 1 thẻ mà local đã xoá mềm → HỒI SINH tombstone
			// local thay vì tạo row mới: giữ `id` (mọi `reviews.card_id` số còn
			// trỏ đúng) và nhận guid của peer để 2 máy hội tụ. Tạo row mới thì
			// 2 máy có 2 thẻ khác id cùng front và lần sync sau nhân đôi.
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
					// PHẢI cập nhật `local` sau khi hồi sinh. Đây là toàn sao
					// của finding B2: lệnh UPDATE vừa ghi `guid = in.GUID` cho
					// row `tombID`, nhưng map `local` vẫn còn khoá `tombGUID` trỏ
					// tới `tombID` với payload CŨ. Incoming tiếp theo mang
					// `tombGUID` (peer cũng có tombstone ấy) sẽ tìm thấy, quyết
					// định UPDATE và ghi `guid = tombGUID, deleted = 1` — **xoá
					// ngược chính thẻ vừa hồi sinh**, lặp lại mãi mỗi lần sync.
					//
					// `local` được cập nhật sau mọi lần ghi ở **5** bảng:
					// `decks`, `cards`, `roadmap_milestones`, `roadmap_resources`,
					// `roadmap_bookmarks`.
					//
					// Ba bảng còn lại (`roadmap_paths`, `roadmap_stages`,
					// `roadmap_topics`) KHÔNG cần, và đây là lý do để không ai
					// "cho đủ" rồi sinh bug mới:
					//  1. Cả 3 có `UNIQUE (guid)`, nên `local` vốn đã đúng: một
					//     incoming guid không thể trùng khoá nào đang có trong
					//     map (trùng thì `Decide` ra Update, không ra Insert).
					//  2. Khi merge ghi, KHÔNG method nào của chúng ghi lại
					//     `guid` — khác `UpsertCard`, chỉ hồi sinh tombstone mới
					//     đổi guid của một row. Không có tình huống "row đổi
					//     guid nhưng map vẫn giữ khoá cũ".
					delete(local, tombGUID)
					local[in.GUID] = in
					adopted[tombGUID] = tombID
					merged.Cards++
					sink.add(&domain.Conflict{
						Table: domain.TableCards, GUID: in.GUID, Winner: domain.WinnerIncoming,
						Detail: fmt.Sprintf("recreate-adopted-tombstone local_guid=%s front=%q id=%d",
							tombGUID, in.Front, tombID),
						At: now,
					})
				}
				continue
			}
			// Front này đã có 1 thẻ SỐNG ở local dưới guid KHÁC, và ta đang
			// nắm rõ thẻ đó (có trong `local`) ⇒ incoming là tombstone cũ của
			// peer, không phải thẻ mới.
			//
			// Phải chặn TRƯỚC khi insert vì `ux_cards_deck_front` là partial
			// (`WHERE deleted = 0`): incoming mang `deleted = 1` thì index KHÔNG
			// chặn, lọt vào DB thành 1 row tombstone rác mà user không tạo.
			// Đây cũng là đường đi của 2 máy cùng gõ tay 1 thẻ (cùng front,
			// guid khác) — trước đây `UpsertCard` tự bắt qua `ON CONFLICT`,
			// giờ chặn sớm hơn nhưng kết quả với user giống hệt: giữ bản
			// local + ghi log.
			if liveID, liveGUID, found, err := s.repo.LiveCardGUIDByFront(ctx, tx, deckID, in.Front); err != nil {
				return nil, fmt.Errorf("tra thẻ sống cùng front %q: %w", in.Front, err)
			} else if found {
				if _, tracked := local[liveGUID]; tracked {
					adopted[in.GUID] = liveID
					sink.add(&domain.Conflict{
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
				// 2 máy cùng tạo 1 thẻ tay: giữ bản local, log lại để user tự
				// xử — im lặng khiến họ tưởng dữ liệu peer bị mất.
				sink.add(&domain.Conflict{
					Table: domain.TableCards, GUID: in.GUID, Winner: domain.WinnerLocal,
					Detail: fmt.Sprintf("unique-live-duplicate kept local front=%q", in.Front),
					At:     now,
				})
			}
			in.ID = id
			local[in.GUID] = in
			merged.Cards++
		}
		sink.add(d.Conflict)
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

// deckIDs đọc lại bản đồ guid→id của deck local trong transaction.
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

// cardIDs đọc lại bản đồ guid→id của card local trong transaction.
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

// mergeRoadmap áp CÙNG luật LWW lên toàn bộ cây `roadmap_*` + bookmarks — đây
// là phần v1 BỎ SÓT (plan M7 ghi rõ), và là lý do context sync cần UnitOfWork
// chung với srs: một lần merge phải chạm cả hai.
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
				// INSERT bị `ON CONFLICT DO NOTHING` bỏ qua: peer có path này
				// nhưng local đã có row trùng UNIQUE khác guid. KHÔNG đếm vào
				// `merged` — báo cáo "đã merge" khi thực tế bị bỏ im lặng là
				// loại bug khiến user tin đã sync xong.
				sink.addSkipped(domain.TableRoadmapPaths, in.GUID, in.Slug, now)
				continue
			}
			ids[in.GUID] = newID
			merged.RoadmapPaths++
		}
		sink.add(d.Conflict)
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
			continue // path cha chưa map được → bỏ (cascade)
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
				sink.addSkipped(domain.TableRoadmapStages, in.GUID, in.Slug, now)
				continue
			}
			ids[in.GUID] = newID
			merged.RoadmapStages++
		}
		sink.add(d.Conflict)
	}
	return ids, nil
}

// stageValues là payload so sánh LWW của stage.
//
// `localDeck` là `deck_guid` của bản LOCAL, dùng khi incoming không mang thông
// tin deck (`DeckGUID == nil`). Bắt buộc: `differs()` so CẢ `len(Values)` lẫn
// từng giá trị, nên thiếu 1 khoá ở một bên sẽ báo "khác nhau" và luật "2 bản
// giống hệt → keep" không bao giờ chạy.
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
				sink.addSkipped(domain.TableRoadmapMilestones, in.GUID, in.Text, now)
				continue
			}
			in.ID = newID
			// Cập nhật `local` sau mọi lần ghi — xem giải thích B2 ở mergeCards.
			local[in.GUID] = in
			merged.RoadmapMilestones++
		}
		sink.add(d.Conflict)
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
				sink.addSkipped(domain.TableRoadmapTopics, in.GUID, in.Title, now)
				continue
			}
			ids[in.GUID] = newID
			merged.RoadmapTopics++
		}
		sink.add(d.Conflict)
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
				sink.addSkipped(domain.TableRoadmapResources, in.GUID, in.Title, now)
				continue
			}
			in.ID = newID
			// Cập nhật `local` sau mọi lần ghi — xem giải thích B2 ở mergeCards.
			local[in.GUID] = in
			merged.RoadmapResources++
		}
		sink.add(d.Conflict)
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
				sink.addSkipped(domain.TableRoadmapBookmarks, in.GUID, in.Title, now)
				continue
			}
			in.ID = newID
			// Cập nhật `local` sau mọi lần ghi — xem giải thích B2 ở mergeCards.
			local[in.GUID] = in
			merged.RoadmapBookmarks++
		}
		sink.add(d.Conflict)
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

// mergeReviews union lịch sử ôn theo guid (KHÔNG LWW — lịch sử append-only) rồi
// REPLAY lịch ôn cho mọi thẻ bị ảnh hưởng.
//
// Replay là bắt buộc: `cards.reps`/`lapses`/`stability`/`due_at` là DẪN XUẤT
// từ lịch sử. Nạp thêm 2 review của peer mà không replay thì reps sai và
// `due_at` lệch — lần sync sau 2 máy lại so LWW trên `updated_at` mà mốc đó
// đã sai từ đầu.
// `adopted` là alias guid cũ → id thẻ hồi sinh (xem `mergeCards`): giữ cho
// `reviews`/`notes` của peer mang guid cũ vẫn trỏ đúng thẻ.
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
			// Thẻ cha đã bị hồi sinh dưới guid khác → dùng alias.
			cardID, ok = adopted[in.CardGUID]
		}
		if !ok {
			continue // card cha chưa map được → bỏ
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
	// Replay theo id tăng dần để thứ tự ghi ổn định giữa 2 lần chạy.
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

// replayCard tính lại lịch 1 thẻ từ toàn bộ lịch sử ôn rồi ghi.
func (s *Service) replayCard(ctx context.Context, tx Tx, cardID int64, cardGUID, now string,
	sink *conflictSink) error {
	revs, err := s.repo.ReviewsOfCard(ctx, tx, cardID)
	if err != nil {
		return fmt.Errorf("đọc lịch sử ôn của card %d: %w", cardID, err)
	}
	if len(revs) == 0 {
		return nil
	}
	// Quy tắc replay nằm ở domain/srs.Replay (lịch 1-3-7-14-30 + FSRS-lite),
	// dùng CHUNG với `srs.Service.RecordReview` — viết lại ở đây là 2 bản lệch
	// nhau sau vài tháng.
	parsed := make([]srsdomain.Review, 0, len(revs))
	for _, r := range revs {
		at, err := time.Parse(time.RFC3339, r.ReviewedAt)
		if err != nil {
			// Review hỏng (timestamp không đọc được) không được làm hỏng cả
			// merge: coi như xảy ra đúng mốc merge, như v1 đã làm.
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
	sink.add(&domain.Conflict{
		Table: domain.TableReviews, GUID: cardGUID, Winner: domain.WinnerLocal,
		Detail: fmt.Sprintf("reps-recomputed từ %d review", len(revs)), At: now,
	})
	return nil
}

// mergeNotes union note theo guid — KHÔNG dedupe nội dung (2 lần ghi cùng nội
// dung là 2 note hợp lệ). `card_id` remap qua card guid; card không resolve
// được → NULL, giữ nội dung thay vì mất.
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
				// Thẻ cha đã bị hồi sinh dưới guid khác → dùng alias.
				id, ok = adopted[in.CardGUID]
			}
			if ok {
				rowToWrite.CardID = &id
			} else {
				// Card không resolve được → để NULL (note tự do) thay vì bỏ
				// note: mất ghi chú luyện tập còn tệ hơn mất liên kết.
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
