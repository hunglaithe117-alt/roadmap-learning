package syncinfra_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ── Helpers dựng dữ liệu 2 máy ─────────────────────────────────────────────

const (
	tsOld  = "2026-09-01T00:00:00Z"
	tsPeer = "2026-09-27T00:00:00Z"
)

// seedDeck chèn 1 deck vào `db` với guid cho trước (giống cả 2 máy đều sinh
// cùng guid nhờ `seedDeckGUID` ổn định).
func seedDeck(t *testing.T, db *gorm.DB, name, guid, updated string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, db.Raw(`INSERT INTO decks (name, lang, created_at, guid, updated_at)
		VALUES (?, 'zh', ?, ?, ?) RETURNING id`, name, updated, guid, updated).Scan(&id).Error)
	return id
}

func seedCard(t *testing.T, db *gorm.DB, deckID int64, front, guid, updated string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, db.Raw(`INSERT INTO cards
		(deck_id, front, back, pinyin, due_at, state, created_at, guid, updated_at)
		VALUES (?, ?, ?, 'ni3', ?, 'new', ?, ?, ?) RETURNING id`,
		deckID, front, "b-"+front, updated, updated, guid, updated).Scan(&id).Error)
	return id
}

func seedReview(t *testing.T, db *gorm.DB, cardID int64, grade int, at, guid string) {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO reviews (card_id, grade, reviewed_at, next_due_at, guid)
		VALUES (?, ?, ?, ?, ?)`, cardID, grade, at, at, guid).Error)
}

func seedNote(t *testing.T, db *gorm.DB, cardID *int64, text, at, guid string) {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO notes (card_id, text, created_at, guid)
		VALUES (?, ?, ?, ?)`, cardID, text, at, guid).Error)
}

func seedPath(t *testing.T, db *gorm.DB, slug, guid, updated string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, db.Raw(`INSERT INTO roadmap_paths (slug, language, title, created_at, guid, updated_at)
		VALUES (?, 'zh', ?, ?, ?, ?) RETURNING id`, slug, "T-"+slug, updated, guid, updated).Scan(&id).Error)
	return id
}

func seedStage(t *testing.T, db *gorm.DB, pathID int64, slug, guid, updated string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, db.Raw(`INSERT INTO roadmap_stages
		(path_id, slug, title, created_at, guid, updated_at) VALUES (?, ?, ?, ?, ?, ?) RETURNING id`,
		pathID, slug, "S-"+slug, updated, guid, updated).Scan(&id).Error)
	return id
}

func seedTopic(t *testing.T, db *gorm.DB, stageID int64, title, guid, updated string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, db.Raw(`INSERT INTO roadmap_topics
		(stage_id, title, created_at, guid, updated_at) VALUES (?, ?, ?, ?, ?) RETURNING id`,
		stageID, title, updated, guid, updated).Scan(&id).Error)
	return id
}

// ── Test ────────────────────────────────────────────────────────────────────

// Kịch bản nền của hầu hết test: mỗi máy tạo deck riêng với guid khác nhau,
// mỗi máy ôn 1 số lần. Merge phải giữ CẢ HAI.
func seedBothMachines(t *testing.T, local, peer *gorm.DB) (localDeck, peerDeck int64) {
	t.Helper()
	localDeck = seedDeck(t, local, "HSK1-local", "d-local", tsOld)
	peerDeck = seedDeck(t, peer, "HSK1-peer", "d-peer", tsPeer)
	return localDeck, peerDeck
}

func Test_merge_unions_both_machines_offline_reviews(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()

	localDeck, peerDeck := seedBothMachines(t, local, peer)
	localCard := seedCard(t, local, localDeck, "你", "c-local", tsOld)
	peerCard := seedCard(t, peer, peerDeck, "好", "c-peer", tsPeer)

	// Mỗi máy ôn offline 2 lần với guid review RIÊNG — đây là dữ liệu phải
	// được GIỮ, không được LWW chọn bên nào.
	for i := 1; i <= 2; i++ {
		seedReview(t, local, localCard, 3, tsOld, "r-local-"+itoa(i))
		seedReview(t, peer, peerCard, 4, tsPeer, "r-peer-"+itoa(i))
	}

	svc := newService(t, local, peer)
	res, err := svc.Sync(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Merged.Decks, "deck của peer phải vào")
	assert.Equal(t, 1, res.Merged.Cards)
	assert.Equal(t, 2, res.Merged.Reviews)

	assert.Equal(t, 2, countRows(t, local, "decks"))
	assert.Equal(t, 2, countRows(t, local, "cards"))
	assert.Equal(t, 4, countRows(t, local, "reviews"),
		"4 lần ôn (2 của mỗi máy) phải còn đủ 4 — union, không LWW chọn bên nào")
}

func Test_merge_keeps_peer_timestamp_as_authoritative(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()

	_, peerDeck := seedBothMachines(t, local, peer)
	seedCard(t, peer, peerDeck, "你", "c-peer", tsPeer)

	svc := newService(t, local, peer)
	res, err := svc.Sync(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, res.Merged.Cards)

	// Mốc LWW phải là mốc PEER, không phải giờ trigger ghi lúc merge.
	// Đây là test bắt đúng lỗi `Omit("updated_at")` mà oracle đã đo: omit nó
	// thì trigger ghi đè và 2 máy so sai vĩnh viễn.
	var updatedAt string
	require.NoError(t, local.Raw("SELECT updated_at FROM cards WHERE guid = 'c-peer'").
		Scan(&updatedAt).Error)
	assert.Equal(t, tsPeer, updatedAt,
		"updated_at phải giữ mốc peer, KHÔNG bị trigger langapp_touch_updated_at ghi đè")
}

func Test_merge_resolves_foreign_keys_through_guid_not_numeric_id(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()

	// Cùng 1 deck (guid giống) nhưng 2 máy tự tăng id khác nhau — đúng tình
	// huống `ATTACH` SQLite của v1 gặp phải.
	localDeck := seedDeck(t, local, "HSK1", "d-same", tsOld)
	peerDeck := seedDeck(t, peer, "HSK1", "d-same", tsPeer)
	// ID 2 máy BẰNG NHAU (mỗi schema có BIGSEQUENCE riêng, cùng bắt đầu từ 1) —
	// đây chính là lý do merge phải remap qua GUID chứ không được tin id số.
	require.Equal(t, localDeck, peerDeck, "test này chỉ có ý nghĩa khi id 2 máy trùng nhau")
	peerCard := seedCard(t, peer, peerDeck, "你", "c-peer", tsPeer)
	seedReview(t, peer, peerCard, 3, tsPeer, "r-peer-1")

	svc := newService(t, local, peer)
	_, err := svc.Sync(ctx)
	require.NoError(t, err)

	// Review của peer phải trỏ tới id card LOCAL, không phải id của máy peer.
	var localCardID int64
	require.NoError(t, local.Raw("SELECT id FROM cards WHERE guid = 'c-peer'").
		Scan(&localCardID).Error)
	assert.Equal(t, peerCard, localCardID, "id số trùng nhau — chỉ GUID mới phân biệt được")

	var cardID int64
	require.NoError(t, local.Raw("SELECT card_id FROM reviews WHERE guid = 'r-peer-1'").Scan(&cardID).Error)
	assert.Equal(t, localCardID, cardID, "FK review phải remap qua guid, không tin id số của peer")
}

func Test_merge_recomputes_reps_from_unified_review_history(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()

	// Cùng 1 thẻ (cùng guid) nhưng 2 máy ôn offline mỗi máy 2 lần.
	localDeck := seedDeck(t, local, "HSK1", "d-same", tsOld)
	peerDeck := seedDeck(t, peer, "HSK1", "d-same", tsPeer)
	seedCard(t, local, localDeck, "你", "c-same", tsOld)
	peerCard := seedCard(t, peer, peerDeck, "你", "c-same", tsPeer)
	for i := 1; i <= 2; i++ {
		seedReview(t, local, peerCard, 3, tsOld, "r-local-"+itoa(i))
		seedReview(t, peer, peerCard, 4, tsPeer, "r-peer-"+itoa(i))
	}

	svc := newService(t, local, peer)
	_, err := svc.Sync(ctx)
	require.NoError(t, err)

	assert.Equal(t, 4, countRows(t, local, "reviews"), "4 lần ôn phải còn đủ 4")

	var reps, lapses int
	var state string
	require.NoError(t, local.Raw(
		"SELECT reps, lapses, state FROM cards WHERE guid = 'c-same'").Row().
		Scan(&reps, &lapses, &state))
	assert.Equal(t, 4, reps, "reps phải replay từ TOÀN BỘ lịch sử, không phải số review của 1 máy")
	assert.Equal(t, 0, lapses)
	assert.Equal(t, "review", state)
}

func Test_merge_is_idempotent_on_second_run(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()
	_, peerDeck := seedBothMachines(t, local, peer)
	seedCard(t, peer, peerDeck, "你", "c-peer", tsPeer)
	seedNote(t, peer, nil, "ghi chú peer", tsPeer, "n-peer")

	svc := newService(t, local, peer)
	first, err := svc.Sync(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, first.Merged.Cards)

	second, err := svc.Sync(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, second.Merged.Cards, "merge lần 2 không được ghi lại")
	assert.Equal(t, 0, second.Merged.Notes)
	assert.Equal(t, 1, countRows(t, local, "cards"))
	assert.Equal(t, 1, countRows(t, local, "notes"))
}

func Test_merge_applies_last_write_wins_by_timestamp(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()

	// Cùng guid, peer sửa sau.
	seedDeck(t, local, "TÊN CŨ", "d-same", tsOld)
	seedDeck(t, peer, "TÊN MỚI TỪ PEER", "d-same", tsPeer)

	svc := newService(t, local, peer)
	res, err := svc.Sync(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Merged.Decks)

	var name, updatedAt string
	require.NoError(t, local.Raw("SELECT name, updated_at FROM decks WHERE guid = 'd-same'").
		Row().Scan(&name, &updatedAt))
	assert.Equal(t, "TÊN MỚI TỪ PEER", name)
	assert.Equal(t, tsPeer, updatedAt)
}

func Test_merge_older_peer_update_does_not_overwrite_local(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()

	seedDeck(t, local, "BẢN MỚI HƠN", "d-same", tsPeer)
	seedDeck(t, peer, "BẢN CŨ HƠN", "d-same", tsOld)

	svc := newService(t, local, peer)
	res, err := svc.Sync(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, res.Merged.Decks, "peer cũ hơn thì không ghi")

	var name string
	require.NoError(t, local.Raw("SELECT name FROM decks WHERE guid = 'd-same'").Scan(&name).Error)
	assert.Equal(t, "BẢN MỚI HƠN", name)
}

func Test_merge_tombstone_wins_when_timestamps_tie(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()

	// Cùng mốc, local còn sống, peer đã xoá mềm → tombstone phải thắng, nếu
	// không thì xoá mềm bị hồi sinh và dữ liệu không bao giờ hội tụ.
	deckID := seedDeck(t, local, "HSK1", "d-same", tsOld)
	cardID := seedCard(t, local, deckID, "你", "c-same", tsOld)
	seedDeck(t, peer, "HSK1", "d-same", tsOld)
	peerCard := seedCard(t, peer, 1, "你", "c-same", tsOld)
	require.NoError(t, peer.Exec("UPDATE cards SET deleted = 1 WHERE id = ?", peerCard).Error)

	svc := newService(t, local, peer)
	res, err := svc.Sync(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Merged.Cards)

	var deleted int
	require.NoError(t, local.Raw("SELECT deleted FROM cards WHERE id = ?", cardID).Scan(&deleted).Error)
	assert.Equal(t, 1, deleted, "xoá mềm phía peer phải thắng khi hai máy cùng mốc")
}

func Test_merge_adopts_local_tombstone_when_peer_recreates_card(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()

	localDeck := seedDeck(t, local, "HSK1", "d-same", tsOld)
	cardID := seedCard(t, local, localDeck, "你好", "c-old", tsOld)
	seedReview(t, local, cardID, 3, tsOld, "r-old")
	require.NoError(t, local.Exec("UPDATE cards SET deleted = 1 WHERE id = ?", cardID).Error)

	// Peer đã xoá rồi tạo lại cùng front với GUID MỚI.
	peerDeck := seedDeck(t, peer, "HSK1", "d-same", tsPeer)
	seedCard(t, peer, peerDeck, "你好", "c-new", tsPeer)

	svc := newService(t, local, peer)
	res, err := svc.Sync(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Merged.Cards)

	// Phải hồi sinh tombstone CŨ (giữ id) chứ không tạo row mới: nếu tạo row
	// mới thì 2 máy có 2 thẻ khác id cùng front và lần sync sau nhân đôi.
	assert.Equal(t, 1, countRows(t, local, "cards"), "chỉ có 1 thẻ, không nhân đôi")
	var guid string
	var deleted int
	require.NoError(t, local.Raw("SELECT guid, deleted FROM cards WHERE id = ?", cardID).
		Row().Scan(&guid, &deleted))
	assert.Equal(t, "c-new", guid, "phải nhận guid của peer để 2 máy hội tụ")
	assert.Equal(t, 0, deleted, "tombstone phải được hồi sinh")
	assert.Equal(t, 1, countRows(t, local, "reviews"), "lịch sử ôn trỏ theo id số phải còn nguyên")
	assert.NotEmpty(t, res.Conflicts, "phải log việc hồi sinh tombstone")
}

func Test_merge_warns_about_clock_skew_without_blocking(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()

	// Peer có mốc nằm 1 NĂM ở tương lai: giờ máy này bị lùi.
	seedDeck(t, peer, "HSK1", "d-future", "2027-09-27T00:00:00Z")

	svc := newService(t, local, peer)
	res, err := svc.Sync(ctx)
	require.NoError(t, err, "clock skew chỉ cảnh báo, KHÔNG chặn merge — sửa giờ là việc của user")
	require.Len(t, res.Warnings, 1)
	assert.Contains(t, res.Warnings[0], "clock-skew")
	assert.Equal(t, 1, countRows(t, local, "decks"), "dữ liệu vẫn phải vào")
}

func Test_merge_rejects_peer_with_different_schema_version(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()

	require.NoError(t, peer.Exec("UPDATE schema_migrations SET version = 99").Error)
	seedDeck(t, peer, "HSK1", "d-x", tsPeer)

	svc := newService(t, local, peer)
	_, err := svc.Sync(ctx)
	require.Error(t, err)
	var appErr interface{ Error() string }
	require.ErrorAs(t, err, &appErr)
	assert.Contains(t, err.Error(), "schema version")
	assert.Equal(t, 0, countRows(t, local, "decks"), "version lệch thì KHÔNG ghi gì")
}

func Test_merge_merges_roadmap_tree_together_with_srs_data(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()

	// Peer có roadmap; local có deck — 2 thứ phải vào CÙNG 1 transaction.
	localDeck := seedDeck(t, local, "HSK1", "d-local", tsOld)
	_ = localDeck
	peerPath := seedPath(t, peer, "zh", "p-1", tsPeer)
	peerStage := seedStage(t, peer, peerPath, "g1", "s-1", tsPeer)
	seedTopic(t, peer, peerStage, "Màn 1", "t-1", tsPeer)

	svc := newService(t, local, peer)
	res, err := svc.Sync(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, res.Merged.RoadmapPaths)
	assert.Equal(t, 1, res.Merged.RoadmapStages)
	assert.Equal(t, 1, res.Merged.RoadmapTopics)

	// FK đúng: topic local phải trỏ stage LOCAL, không phải id của máy peer.
	var localStageID, localTopicID int64
	require.NoError(t, local.Raw("SELECT id FROM roadmap_stages WHERE guid = 's-1'").
		Scan(&localStageID).Error)
	require.NoError(t, local.Raw("SELECT id FROM roadmap_topics WHERE guid = 't-1'").
		Scan(&localTopicID).Error)
	assert.Equal(t, peerStage, localStageID, "id 2 máy trùng nhau — chỉ GUID mới phân biệt được")
	var stageID int64
	require.NoError(t, local.Raw("SELECT stage_id FROM roadmap_topics WHERE id = ?", localTopicID).
		Scan(&stageID).Error)
	assert.Equal(t, localStageID, stageID)
}

func Test_merge_rolls_back_roadmap_and_srs_together_on_midway_failure(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()

	seedDeck(t, peer, "HSK1", "d-peer", tsPeer)
	peerPath := seedPath(t, peer, "zh", "p-1", tsPeer)
	seedStage(t, peer, peerPath, "g1", "s-1", tsPeer)
	seedNote(t, peer, nil, "ghi chú peer", tsPeer, "n-peer")

	// Chặn INSERT notes sau khi decks + roadmap đã ghi: nếu không có 1
	// transaction chung, DB local sẽ còn deck + roadmap mà mất note.
	require.NoError(t, peer.Exec(`UPDATE schema_migrations SET version = 4`).Error)
	require.NoError(t, local.Exec(`
		CREATE OR REPLACE FUNCTION t_block_note() RETURNS trigger
		LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'blocked'; END; $$`).Error)
	require.NoError(t, local.Exec(`
		CREATE TRIGGER t_block_note_ins BEFORE INSERT ON notes
		FOR EACH ROW EXECUTE FUNCTION t_block_note()`).Error)

	svc := newService(t, local, peer)
	_, err := svc.Sync(ctx)
	require.Error(t, err, "lỗi giữa chừng phải làm cả merge thất bại")

	assert.Equal(t, 0, countRows(t, local, "decks"), "deck đã ghi trước phải bị rollback")
	assert.Equal(t, 0, countRows(t, local, "roadmap_paths"), "roadmap phải bị rollback CÙNG")
	assert.Equal(t, 0, countRows(t, local, "roadmap_stages"))
}

func Test_merge_rolls_back_when_conflict_log_write_fails(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()

	seedDeck(t, local, "CŨ", "d-same", tsPeer)
	seedDeck(t, peer, "MỚI", "d-same", tsPeer)
	require.NoError(t, local.Exec(`
		CREATE OR REPLACE FUNCTION t_block_conflict() RETURNS trigger
		LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'blocked'; END; $$`).Error)
	require.NoError(t, local.Exec(`
		CREATE TRIGGER t_block_conflict_ins BEFORE INSERT ON sync_conflicts
		FOR EACH ROW EXECUTE FUNCTION t_block_conflict()`).Error)

	svc := newService(t, local, peer)
	// Ghi log conflict thất bại KHÔNG được làm hỏng merge: dữ liệu hội tụ là
	// mục tiêu chính, log chỉ để user xem lại.
	_, err := svc.Sync(ctx)
	require.NoError(t, err)
}

func Test_merge_notes_keep_prefix_and_card_link(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()

	peerDeck := seedDeck(t, peer, "HSK1", "d-peer", tsPeer)
	peerCard := seedCard(t, peer, peerDeck, "你", "c-peer", tsPeer)
	seedNote(t, peer, &peerCard, `ERR|{"expected":"a","transcript":"b","wrong":["x"]}`, tsPeer, "n-err")
	seedNote(t, peer, nil, `THIEU|{"session":"2026-09-28","scores":{}}`, tsPeer, "n-thieu")

	svc := newService(t, local, peer)
	res, err := svc.Sync(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Merged.Notes)

	var localCardID int64
	require.NoError(t, local.Raw("SELECT id FROM cards WHERE guid = 'c-peer'").
		Scan(&localCardID).Error)
	var cardID *int64
	var text string
	require.NoError(t, local.Raw("SELECT card_id, text FROM notes WHERE guid = 'n-err'").
		Row().Scan(&cardID, &text))
	require.NotNil(t, cardID, "note gắn thẻ phải trỏ tới card LOCAL")
	assert.Equal(t, localCardID, *cardID)
	assert.Contains(t, text, "ERR|", "prefix reserved phải giữ nguyên khi merge")

	var thieuCard *int64
	require.NoError(t, local.Raw("SELECT card_id FROM notes WHERE guid = 'n-thieu'").
		Scan(&thieuCard).Error)
	assert.Nil(t, thieuCard, "checklist THIEU không gắn thẻ")
}

func Test_merge_ignores_dict_and_version_tables_from_snapshot(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()

	require.NoError(t, peer.Exec("INSERT INTO dict (hanzi, pinyin, nghia) VALUES ('你', 'ni3', 'bạn')").Error)
	require.NoError(t, peer.Exec("INSERT INTO en_dict (lang, term, reading, gloss) VALUES ('en','x','/y/','z')").Error)
	before := countRows(t, local, "goose_db_version")

	svc := newService(t, local, peer)
	_, err := svc.Sync(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, countRows(t, local, "dict"),
		"dict là dữ liệu tĩnh 2 máy đã giống nhau; nạp sang sẽ đè nghĩa user sửa tay")
	assert.Equal(t, 0, countRows(t, local, "en_dict"))
	assert.Equal(t, before, countRows(t, local, "goose_db_version"),
		"goose_db_version là version của DB local, không bao giờ lấy từ peer")
}

func Test_merge_writes_last_sync_at_and_conflict_log(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()

	// Cả 2 vế đều đổi sau last_sync ⇒ phải log xung đột.
	require.NoError(t, local.Exec(
		"INSERT INTO sync_meta (k, v) VALUES ('last_sync_at', '2026-09-01T00:00:00Z') "+
			"ON CONFLICT (k) DO UPDATE SET v = excluded.v").Error)
	seedDeck(t, local, "A", "d-same", tsPeer)
	seedDeck(t, peer, "B", "d-same", tsPeer)

	svc := newService(t, local, peer)
	res, err := svc.Sync(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, res.Conflicts)
	assert.Equal(t, fixedNow.Format(time.RFC3339), res.LastSyncAt)
	assert.Equal(t, len(res.Conflicts), countRows(t, local, "sync_conflicts"),
		"log xung đột phải được ghi trong DB để user xem lại")

	status, err := svc.Status(ctx)
	require.NoError(t, err)
	assert.True(t, status.Enabled)
	assert.Equal(t, res.LastSyncAt, status.LastSyncAt)
	assert.Equal(t, len(res.Conflicts), status.ConflictCount)
}

func Test_merge_rejects_duplicate_live_card_from_both_machines(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()

	// 2 máy cùng tạo 1 thẻ tay cùng front nhưng GUID KHÁC nhau (người dùng
	// gõ tay ở cả 2 máy) → `ux_cards_deck_front` sẽ chặn. Merge phải giữ bản
	// local + ghi log, KHÔNG báo lỗi làm hỏng cả merge.
	localDeck := seedDeck(t, local, "HSK1", "d-same", tsOld)
	seedCard(t, local, localDeck, "你好", "c-local", tsOld)
	peerDeck := seedDeck(t, peer, "HSK1", "d-same", tsPeer)
	seedCard(t, peer, peerDeck, "你好", "c-peer", tsPeer)

	svc := newService(t, local, peer)
	res, err := svc.Sync(ctx)
	require.NoError(t, err, "trùng front do 2 máy tạo tay không được làm hỏng merge")
	assert.Equal(t, 1, countRows(t, local, "cards"), "không được tạo row thứ 2 cùng front")
	assert.NotEmpty(t, res.Conflicts, "phải log lại việc giữ bản local")
}

func itoa(n int) string { return strconv.Itoa(n) }
