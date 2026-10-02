package sync

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "langapp/internal/domain/sync"
)

const (
	tsOld = "2026-09-01T00:00:00Z"
	tsNew = "2026-09-20T00:00:00Z"
	// tsMid nằm giữa tsOld và tsPeer — dùng cho tombstone của peer MỚI HƠN bản
	// tombstone local (xem remediation_test.go).
	tsMid  = "2026-09-15T00:00:00Z"
	tsPeer = "2026-09-27T00:00:00Z"
)

// ── Version ─────────────────────────────────────────────────────────────────

func Test_merge_rejects_peer_with_different_schema_version(t *testing.T) {
	repo := newFakeRepo()
	repo.peerV = 3
	svc, uow := newService(repo, domain.PeerSnapshot{SchemaVersion: 3})

	_, err := svc.Merge(context.Background(), domain.PeerSnapshot{SchemaVersion: 3})
	require.Error(t, err)
	var appErr *Error
	require.ErrorAs(t, err, &appErr)
	assert.Equal(t, StatusBadRequest, appErr.Status)
	assert.Zero(t, uow.inTx, "phải chặn TRƯỚC khi mở transaction")
	assert.Empty(t, repo.decks, "không được ghi gì khi version lệch")
}

// ── LWW ─────────────────────────────────────────────────────────────────────

func Test_merge_inserts_row_missing_locally(t *testing.T) {
	repo := newFakeRepo()
	svc, _ := newService(repo, domain.PeerSnapshot{SchemaVersion: 4})
	svc.repo.SetLastSyncAt(context.Background(), nil, tsOld)

	res, err := svc.Merge(context.Background(), domain.PeerSnapshot{
		SchemaVersion: 4,
		Rows: []domain.Row{
			snapRow(domain.TableDecks, "d1", "", tsPeer, 0,
				map[string]string{"name": "HSK1", "lang": "zh"}),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Merged.Decks)
	assert.Len(t, repo.decks, 1)
}

func Test_merge_keeps_local_when_peer_is_older(t *testing.T) {
	repo := newFakeRepo()
	repo.lastSync = tsOld
	_, err := repo.UpsertDeck(context.Background(), nil, DeckRow{
		GUID: "d1", Name: "TÊN MỚI", Lang: "zh", CreatedAt: tsNew, UpdatedAt: tsNew,
	}, false)
	require.NoError(t, err)
	svc, _ := newService(repo, domain.PeerSnapshot{SchemaVersion: 4})

	res, err := svc.Merge(context.Background(), domain.PeerSnapshot{
		SchemaVersion: 4,
		Rows: []domain.Row{
			snapRow(domain.TableDecks, "d1", "", tsOld, 0,
				map[string]string{"name": "TÊN CŨ", "lang": "zh"}),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 0, res.Merged.Decks, "peer cũ hơn thì không ghi")
	assert.Equal(t, "TÊN MỚI", repo.decks["d1"].payload.(DeckRow).Name)
}

func Test_merge_applies_newer_peer_update(t *testing.T) {
	repo := newFakeRepo()
	repo.lastSync = tsOld
	_, err := repo.UpsertDeck(context.Background(), nil, DeckRow{
		GUID: "d1", Name: "CŨ", Lang: "zh", CreatedAt: tsOld, UpdatedAt: tsOld,
	}, false)
	require.NoError(t, err)
	svc, _ := newService(repo, domain.PeerSnapshot{SchemaVersion: 4})

	res, err := svc.Merge(context.Background(), domain.PeerSnapshot{
		SchemaVersion: 4,
		Rows: []domain.Row{
			snapRow(domain.TableDecks, "d1", "", tsPeer, 0,
				map[string]string{"name": "MỚI TỪ PEER", "lang": "zh"}),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Merged.Decks)
	got := repo.decks["d1"].payload.(DeckRow)
	assert.Equal(t, "MỚI TỪ PEER", got.Name)
	assert.Equal(t, tsPeer, got.UpdatedAt, "mốc peer phải sống sót, không bị trigger ghi đè")
}

func Test_merge_tombstone_wins_on_equal_timestamp(t *testing.T) {
	repo := newFakeRepo()
	repo.lastSync = tsOld
	_, err := repo.UpsertDeck(context.Background(), nil, DeckRow{
		GUID: "d1", Name: "X", Lang: "zh", CreatedAt: tsOld, UpdatedAt: tsOld,
	}, false)
	require.NoError(t, err)
	svc, _ := newService(repo, domain.PeerSnapshot{SchemaVersion: 4})

	// Cùng mốc, peer đã xoá mềm → tombstone THẮNG, nếu không thì xoá mềm bị
	// hồi sinh ở máy này và dữ liệu không bao giờ hội tụ.
	res, err := svc.Merge(context.Background(), domain.PeerSnapshot{
		SchemaVersion: 4,
		Rows: []domain.Row{
			snapRow(domain.TableDecks, "d1", "", tsOld, 1,
				map[string]string{"name": "X", "lang": "zh"}),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Merged.Decks)
	assert.Equal(t, 1, repo.decks["d1"].payload.(DeckRow).Deleted)
}

func Test_merge_is_idempotent_when_run_twice(t *testing.T) {
	repo := newFakeRepo()
	svc, _ := newService(repo, domain.PeerSnapshot{SchemaVersion: 4})
	snap := domain.PeerSnapshot{
		SchemaVersion: 4,
		Rows: []domain.Row{
			snapRow(domain.TableDecks, "d1", "", tsPeer, 0,
				map[string]string{"name": "HSK1", "lang": "zh"}),
			snapRow(domain.TableCards, "c1", "d1", tsPeer, 0, map[string]string{
				"front": "你", "back": "bạn", "pinyin": "ni3", "due_at": tsPeer, "state": "new",
			}),
		},
	}

	first, err := svc.Merge(context.Background(), snap)
	require.NoError(t, err)
	assert.Equal(t, 1, first.Merged.Decks)
	assert.Equal(t, 1, first.Merged.Cards)

	second, err := svc.Merge(context.Background(), snap)
	require.NoError(t, err)
	assert.Equal(t, 0, second.Merged.Decks, "merge lần 2 không được ghi lại")
	assert.Equal(t, 0, second.Merged.Cards)
	assert.Len(t, repo.decks, 1)
	assert.Len(t, repo.cards, 1)
}

// ── Remap guid → id ─────────────────────────────────────────────────────────

func Test_merge_remaps_card_deck_id_through_guid(t *testing.T) {
	repo := newFakeRepo()
	svc, _ := newService(repo, domain.PeerSnapshot{SchemaVersion: 4})

	_, err := svc.Merge(context.Background(), domain.PeerSnapshot{
		SchemaVersion: 4,
		Rows: []domain.Row{
			snapRow(domain.TableDecks, "d1", "", tsPeer, 0,
				map[string]string{"name": "HSK1", "lang": "zh"}),
			snapRow(domain.TableCards, "c1", "d1", tsPeer, 0, map[string]string{
				"front": "你", "back": "bạn", "state": "new", "due_at": tsPeer,
			}),
		},
	})
	require.NoError(t, err)
	card := repo.cards["c1"].payload.(CardRow)
	require.NotNil(t, card.DeckID, "card phải trỏ tới deck local")
	assert.Equal(t, repo.decks["d1"].id, *card.DeckID)
}

func Test_merge_drops_card_whose_deck_cannot_be_resolved(t *testing.T) {
	repo := newFakeRepo()
	svc, _ := newService(repo, domain.PeerSnapshot{SchemaVersion: 4})

	res, err := svc.Merge(context.Background(), domain.PeerSnapshot{
		SchemaVersion: 4,
		Rows: []domain.Row{
			// DeckGUID rỗng: snapshot hỏng, hoặc deck cha đã bị bỏ.
			snapRow(domain.TableCards, "c1", "", tsPeer, 0, map[string]string{
				"front": "你", "back": "bạn", "state": "new", "due_at": tsPeer,
			}),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 0, res.Merged.Cards)
	assert.Empty(t, repo.cards, "card mồ côi phải bị bỏ, không tạo row hỏng")
}

func Test_merge_adopts_local_tombstone_when_peer_recreates_card(t *testing.T) {
	repo := newFakeRepo()
	_, err := repo.UpsertDeck(context.Background(), nil,
		DeckRow{GUID: "d1", Name: "HSK1", Lang: "zh", CreatedAt: tsOld, UpdatedAt: tsOld}, false)
	require.NoError(t, err)
	seedLocalCard(repo, "c-old", "d1", "你好", 1, tsOld)
	tombID := repo.cards["c-old"].id

	svc, _ := newService(repo, domain.PeerSnapshot{SchemaVersion: 4})
	res, err := svc.Merge(context.Background(), domain.PeerSnapshot{
		SchemaVersion: 4,
		Rows: []domain.Row{
			snapRow(domain.TableDecks, "d1", "", tsPeer, 0,
				map[string]string{"name": "HSK1", "lang": "zh"}),
			// Peer đã xoá rồi tạo lại cùng front với GUID MỚI.
			snapRow(domain.TableCards, "c-new", "d1", tsPeer, 0, map[string]string{
				"front": "你好", "back": "xin chào", "state": "new", "due_at": tsPeer,
			}),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Merged.Cards)
	assert.Equal(t, tombID, repo.cards["c-new"].id, "phải hồi sinh tombstone, giữ nguyên id")
	assert.Equal(t, 0, repo.cards["c-new"].payload.(CardRow).Deleted)
	assert.NotEmpty(t, res.Conflicts, "phải log lại việc hồi sinh tombstone")
}

// ── Reviews (append + replay) ───────────────────────────────────────────────

func Test_merge_unions_reviews_and_replays_schedule(t *testing.T) {
	repo := newFakeRepo()
	_, err := repo.UpsertDeck(context.Background(), nil,
		DeckRow{GUID: "d1", Name: "HSK1", Lang: "zh", CreatedAt: tsOld, UpdatedAt: tsOld}, false)
	require.NoError(t, err)
	seedLocalCard(repo, "c1", "d1", "你好", 0, tsOld)

	// Local đã có 1 review (offline trước) với guid khác.
	cardID := repo.cards["c1"].id
	_, err = repo.AppendReview(context.Background(), nil, ReviewRow{
		GUID: "r-local", CardID: &cardID, Grade: 3, ReviewedAt: tsOld, NextDueAt: tsOld,
	})
	require.NoError(t, err)

	svc, _ := newService(repo, domain.PeerSnapshot{SchemaVersion: 4})
	res, err := svc.Merge(context.Background(), domain.PeerSnapshot{
		SchemaVersion: 4,
		Rows: []domain.Row{
			snapRow(domain.TableDecks, "d1", "", tsOld, 0,
				map[string]string{"name": "HSK1", "lang": "zh"}),
			snapRow(domain.TableCards, "c1", "d1", tsOld, 0, map[string]string{
				"front": "你好", "back": "xin chào", "state": "new", "due_at": tsOld,
			}),
			snapRow(domain.TableReviews, "r-local", "c1", tsOld, 0, map[string]string{
				"grade": "3", "reviewed_at": tsOld, "next_due_at": tsOld,
			}),
			snapRow(domain.TableReviews, "r-peer", "c1", tsPeer, 0, map[string]string{
				"grade": "4", "reviewed_at": tsPeer, "next_due_at": tsPeer,
			}),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Merged.Reviews, "chỉ review CHƯA có mới được nạp")
	assert.Len(t, repo.reviews, 2, "2 lần ôn phải còn cả 2 — không được mất dòng")

	card := repo.cards["c1"].payload.(CardRow)
	assert.Equal(t, 2, card.Reps, "reps phải replay từ toàn bộ lịch sử")
	assert.Equal(t, "review", card.State)
	assert.Equal(t, fixedNow.UTC().Format(time.RFC3339), card.UpdatedAt,
		"replay phải set updated_at tường minh bằng mốc merge")
}

func Test_merge_keeps_note_when_its_card_cannot_be_resolved(t *testing.T) {
	repo := newFakeRepo()
	svc, _ := newService(repo, domain.PeerSnapshot{SchemaVersion: 4})

	res, err := svc.Merge(context.Background(), domain.PeerSnapshot{
		SchemaVersion: 4,
		Rows: []domain.Row{
			snapRow(domain.TableNotes, "n1", "c-khong-ton-tai", tsPeer, 0,
				map[string]string{"text": `ERR|{"expected":"x","transcript":"y","wrong":[]}`}),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Merged.Notes, "mất ghi chú luyện tập còn tệ hơn mất liên kết thẻ")
	note := repo.notes["n1"].payload.(NoteRow)
	assert.Nil(t, note.CardID, "card không resolve được → NULL")
}

func Test_merge_unions_notes_by_guid_without_deduping_content(t *testing.T) {
	repo := newFakeRepo()
	_, err := repo.AppendNote(context.Background(), nil, NoteRow{
		GUID: "n-local", Text: "ghi chú 1", CreatedAt: tsOld,
	})
	require.NoError(t, err)

	svc, _ := newService(repo, domain.PeerSnapshot{SchemaVersion: 4})
	res, err := svc.Merge(context.Background(), domain.PeerSnapshot{
		SchemaVersion: 4,
		Rows: []domain.Row{
			snapRow(domain.TableNotes, "n-local", "", tsPeer, 0, map[string]string{"text": "ghi chú 1"}),
			snapRow(domain.TableNotes, "n-peer", "", tsPeer, 0, map[string]string{"text": "ghi chú 1"}),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Merged.Notes, "2 note trùng NỘI DUNG là 2 note hợp lệ, chỉ dedupe theo guid")
	assert.Len(t, repo.notes, 2)
}

// ── Roadmap ─────────────────────────────────────────────────────────────────

func Test_merge_merges_roadmap_tree_in_dependency_order(t *testing.T) {
	repo := newFakeRepo()
	svc, _ := newService(repo, domain.PeerSnapshot{SchemaVersion: 4})

	res, err := svc.Merge(context.Background(), domain.PeerSnapshot{
		SchemaVersion: 4,
		Rows: []domain.Row{
			snapRow(domain.TableRoadmapPaths, "p1", "", tsPeer, 0, map[string]string{
				"slug": "zh", "language": "zh", "title": "Tiếng Trung", "overview": "",
				"is_builtin": "1", "terrain": "", "direction": "",
			}),
			snapRow(domain.TableRoadmapStages, "s1", "p1", tsPeer, 0, map[string]string{
				"slug": "g1", "title": "Giai đoạn 1", "status": "not_started",
				"terrain": "meadow", "direction": "up",
			}),
			snapRow(domain.TableRoadmapTopics, "t1", "s1", tsPeer, 0, map[string]string{
				"title": "Màn 1", "status": "done", "is_optional": "0",
				"completed_at": tsPeer, "map_x": "0.5", "map_y": "0.25",
			}),
			snapRow(domain.TableRoadmapResources, "res1", "t1", tsPeer, 0, map[string]string{
				"title": "Bài tập", "kind": "exercise", "note": "", "position": "0",
			}),
			snapRow(domain.TableRoadmapMilestones, "m1", "s1", tsPeer, 0, map[string]string{
				"text": "Hoàn thành G1", "position": "0",
			}),
			snapRow(domain.TableRoadmapBookmarks, "b1", "", tsPeer, 0, map[string]string{
				"title": "Link hay", "status": "to_read", "tags": "", "note": "",
			}),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Merged.RoadmapPaths)
	assert.Equal(t, 1, res.Merged.RoadmapStages)
	assert.Equal(t, 1, res.Merged.RoadmapTopics)
	assert.Equal(t, 1, res.Merged.RoadmapResources)
	assert.Equal(t, 1, res.Merged.RoadmapMilestones)
	assert.Equal(t, 1, res.Merged.RoadmapBookmarks)
	assert.Len(t, repo.paths, 1)
	assert.Len(t, repo.stages, 1)
	assert.Len(t, repo.topics, 1)
	assert.Len(t, repo.res, 1)
	assert.Len(t, repo.ms, 1)
	assert.Len(t, repo.bm, 1)
}

func Test_merge_drops_topic_whose_stage_cannot_be_resolved(t *testing.T) {
	repo := newFakeRepo()
	svc, _ := newService(repo, domain.PeerSnapshot{SchemaVersion: 4})

	res, err := svc.Merge(context.Background(), domain.PeerSnapshot{
		SchemaVersion: 4,
		Rows: []domain.Row{
			snapRow(domain.TableRoadmapTopics, "t1", "s-khong-ton-tai", tsPeer, 0, map[string]string{
				"title": "Màn", "status": "not_started", "is_optional": "0",
			}),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 0, res.Merged.RoadmapTopics)
	assert.Empty(t, repo.topics, "node mồ côi phải bị bỏ")
}

func Test_merge_preserves_optional_and_map_coordinates(t *testing.T) {
	repo := newFakeRepo()
	svc, _ := newService(repo, domain.PeerSnapshot{SchemaVersion: 4})

	_, err := svc.Merge(context.Background(), domain.PeerSnapshot{
		SchemaVersion: 4,
		Rows: []domain.Row{
			snapRow(domain.TableRoadmapPaths, "p1", "", tsPeer, 0,
				map[string]string{"slug": "zh", "language": "zh", "title": "T"}),
			snapRow(domain.TableRoadmapStages, "s1", "p1", tsPeer, 0,
				map[string]string{"slug": "g1", "title": "G1", "status": "not_started"}),
			snapRow(domain.TableRoadmapTopics, "t1", "s1", tsPeer, 0, map[string]string{
				"title": "Tham khảo", "status": "not_started", "is_optional": "1",
				"map_x": "0.75", "map_y": "0.125",
			}),
		},
	})
	require.NoError(t, err)
	topic := repo.topics["t1"].payload.(TopicRow)
	assert.Equal(t, 1, topic.IsOptional)
	require.NotNil(t, topic.MapX, "map_x phải giữ NULL ≠ 0")
	assert.InDelta(t, 0.75, *topic.MapX, 1e-9)
	assert.InDelta(t, 0.125, *topic.MapY, 1e-9)
}

// ── Transaction ─────────────────────────────────────────────────────────────

func Test_merge_rolls_back_every_table_when_a_later_write_fails(t *testing.T) {
	repo := newFakeRepo()
	// Ghi `cards` OK, `notes` fail ⇒ không bảng nào trước đó được giữ lại.
	repo.failOn = "notes"
	svc, uow := newService(repo, domain.PeerSnapshot{SchemaVersion: 4})

	_, err := svc.Merge(context.Background(), domain.PeerSnapshot{
		SchemaVersion: 4,
		Rows: []domain.Row{
			snapRow(domain.TableDecks, "d1", "", tsPeer, 0,
				map[string]string{"name": "HSK1", "lang": "zh"}),
			snapRow(domain.TableCards, "c1", "d1", tsPeer, 0, map[string]string{
				"front": "你", "back": "bạn", "state": "new", "due_at": tsPeer,
			}),
			snapRow(domain.TableRoadmapPaths, "p1", "", tsPeer, 0,
				map[string]string{"slug": "zh", "language": "zh", "title": "T"}),
			snapRow(domain.TableNotes, "n1", "", tsPeer, 0,
				map[string]string{"text": "ghi chú"}),
		},
	})
	require.Error(t, err)
	assert.Equal(t, 1, uow.inTx, "toàn bộ merge phải nằm trong ĐÚNG 1 transaction")
	assert.Empty(t, repo.decks, "deck đã ghi trước phải bị rollback")
	assert.Empty(t, repo.cards)
	assert.Empty(t, repo.paths)
	assert.Empty(t, repo.lastSync, "last_sync_at chỉ được ghi khi merge thành công")
}

func Test_merge_updates_last_sync_at_only_on_success(t *testing.T) {
	repo := newFakeRepo()
	svc, _ := newService(repo, domain.PeerSnapshot{SchemaVersion: 4})

	res, err := svc.Merge(context.Background(), domain.PeerSnapshot{SchemaVersion: 4})
	require.NoError(t, err)
	assert.NotEmpty(t, res.LastSyncAt)
	assert.Equal(t, res.LastSyncAt, repo.lastSync)
}

// ── Cảnh báo ────────────────────────────────────────────────────────────────

func Test_merge_warns_when_peer_snapshot_is_from_the_future(t *testing.T) {
	repo := newFakeRepo()
	svc, _ := newService(repo, domain.PeerSnapshot{SchemaVersion: 4})

	res, err := svc.Merge(context.Background(), domain.PeerSnapshot{
		SchemaVersion: 4, MaxUpdatedAt: "2027-01-01T00:00:00Z",
	})
	require.NoError(t, err, "clock skew chỉ CẢNH BÁO, không chặn merge")
	require.Len(t, res.Warnings, 1)
	assert.Contains(t, res.Warnings[0], "clock-skew")
}

func Test_merge_does_not_warn_without_timestamp(t *testing.T) {
	repo := newFakeRepo()
	svc, _ := newService(repo, domain.PeerSnapshot{SchemaVersion: 4})

	res, err := svc.Merge(context.Background(), domain.PeerSnapshot{SchemaVersion: 4})
	require.NoError(t, err)
	assert.Empty(t, res.Warnings)
}

// ── Bảng bị bỏ qua ─────────────────────────────────────────────────────────

func Test_skip_tables_exclude_dict_and_version_books(t *testing.T) {
	// dict/en_dict là dữ liệu tĩnh 2 máy đã giống nhau; version books là
	// version của chính DB mỗi máy. Ghi đè chúng sẽ phá cả 2.
	for _, table := range []string{"dict", "en_dict", "schema_migrations", "goose_db_version"} {
		assert.Contains(t, SkipTables, table)
	}
}

func Test_merge_never_reads_dict_from_snapshot(t *testing.T) {
	repo := newFakeRepo()
	svc, _ := newService(repo, domain.PeerSnapshot{SchemaVersion: 4})

	// Snapshot cố chứa row dict/en_dict (snapshot bị dựng sai) — merge phải
	// bỏ qua, không ghi vào bảng của mình.
	_, err := svc.Merge(context.Background(), domain.PeerSnapshot{
		SchemaVersion: 4,
		Rows: []domain.Row{{
			Table: domain.Table("dict"), GUID: "x", UpdatedAt: tsPeer,
			Values: map[string]string{"hanzi": "你", "pinyin": "ni3", "nghia": "bạn"},
		}},
	})
	require.NoError(t, err)
	assert.Empty(t, repo.decks)
	assert.Empty(t, repo.cards)
}

// ── Status / Conflicts ──────────────────────────────────────────────────────

func Test_status_reports_last_sync_and_conflict_count(t *testing.T) {
	repo := newFakeRepo()
	repo.lastSync = tsOld
	repo.conflicts = []domain.Conflict{{Table: domain.TableDecks}}
	svc, _ := newService(repo, domain.PeerSnapshot{SchemaVersion: 4})

	got, err := svc.Status(context.Background())
	require.NoError(t, err)
	assert.True(t, got.Enabled, "merge luôn sẵn sàng, không có cờ tắt/mở")
	assert.Equal(t, MergeStrategy, got.Strategy)
	assert.Equal(t, tsOld, got.LastSyncAt)
	assert.Equal(t, 1, got.ConflictCount)
}

func Test_conflicts_are_persisted_for_user_to_review(t *testing.T) {
	repo := newFakeRepo()
	repo.lastSync = tsOld
	_, err := repo.UpsertDeck(context.Background(), nil, DeckRow{
		GUID: "d1", Name: "A", Lang: "zh", CreatedAt: tsPeer, UpdatedAt: tsPeer,
	}, false)
	require.NoError(t, err)
	repo.decks["d1"] = stored{id: repo.decks["d1"].id, payload: DeckRow{
		GUID: "d1", Name: "B", Lang: "zh", CreatedAt: tsPeer, UpdatedAt: tsPeer}}

	svc, _ := newService(repo, domain.PeerSnapshot{SchemaVersion: 4})
	res, err := svc.Merge(context.Background(), domain.PeerSnapshot{
		SchemaVersion: 4,
		Rows: []domain.Row{
			snapRow(domain.TableDecks, "d1", "", tsPeer, 0,
				map[string]string{"name": "C", "lang": "zh"}),
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, res.Conflicts, "cả 2 vế đều đổi sau last_sync thì phải log xung đột")
	assert.Len(t, repo.conflicts, len(res.Conflicts), "log phải nằm trong transaction")
}
