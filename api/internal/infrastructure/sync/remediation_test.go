package syncinfra_test

// Test cho remediation cổng Oracle M3 (5 finding). Mỗi test dựng lại ĐÚNG kịch
// bản Oracle đã reproduce, không phải biến thể dễ hơn.
//
//	B1  merge xoá `roadmap_stages.deck_id` ⇒ chết feature A1
//	B2  hồi sinh tombstone bị chính lần merge ghi đè ⇒ oscillation
//	H3  đọc pool BÊN TRONG transaction merge
//	H4  `txOf` rơi về pool im lặng khi tx sai kiểu
//	DRY chốt 4 danh sách cột phải khớp tay

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	contentapp "langapp/internal/application/content"
	practiceapp "langapp/internal/application/practice"
	syncapp "langapp/internal/application/sync"
	contentinfra "langapp/internal/infrastructure/content"
	practiceinfra "langapp/internal/infrastructure/practice"
	syncinfra "langapp/internal/infrastructure/sync"
)

// wrongTx là `app.Tx` (`= any`) KHÔNG phải handle do repository tạo — mô phỏng
// việc lấy nhầm handle giữa 2 context (rất dễ xảy ra khi 7 context cùng dùng
// kiểu `app.Tx` trống).
type wrongTx struct{ context.Context }

// ═══════════════════════════════════════════════════════════════════════════
// B1 — merge xoá `roadmap_stages.deck_id` ⇒ chết feature A1
// ═══════════════════════════════════════════════════════════════════════════

// seedStageWithDeck chèn stage; `deckID == nil` ⇒ stage chưa gắn deck.
func seedStageWithDeck(t *testing.T, db *gorm.DB, pathID int64, deckID *int64, slug, guid, updated string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, db.Raw(`INSERT INTO roadmap_stages
		(path_id, deck_id, slug, title, created_at, guid, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) RETURNING id`,
		pathID, deckID, slug, "S-"+slug, updated, guid, updated).Scan(&id).Error)
	return id
}

func stageDeckID(t *testing.T, db *gorm.DB, guid string) *int64 {
	t.Helper()
	var out *int64
	require.NoError(t, db.Raw("SELECT deck_id FROM roadmap_stages WHERE guid = ?", guid).
		Row().Scan(&out))
	return out
}

// Kịch bản đúng như Oracle: local có stage gắn deck, peer SỬA stage đó với
// `updated_at` mới hơn. Trước fix: sau merge `deck_id = NULL`, không conflict
// log, không lỗi — âm thầm. "Bấm stage nhảy thẳng `/review`" chết sau sync
// đầu tiên.
//
// Side-effect thứ 2 Oracle nêu: `differs()` luôn true nên luật "2 bản giống
// hệt → keep" không bao giờ chạy. Test thứ 2 ở `application/sync` phủ cái đó.
func Test_B1_merge_keeps_stage_deck_id_when_peer_edits_same_stage(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()

	localDeck := seedDeck(t, local, "HSK1", "d-same", tsOld)
	localPath := seedPath(t, local, "zh", "p-same", tsOld)
	seedStageWithDeck(t, local, localPath, &localDeck, "s1", "st-same", tsOld)

	peerDeck := seedDeck(t, peer, "HSK1", "d-same", tsPeer)
	peerPath := seedPath(t, peer, "zh", "p-same", tsPeer)
	seedStageWithDeck(t, peer, peerPath, &peerDeck, "s1", "st-same", tsPeer)
	require.NoError(t, peer.Exec("UPDATE roadmap_stages SET status = 'done' WHERE slug = 's1'").Error)

	svc := newService(t, local, peer)
	_, err := svc.Sync(ctx)
	require.NoError(t, err)

	got := stageDeckID(t, local, "st-same")
	require.NotNil(t, got, "B1: deck_id bị xoá NULL — chết feature A1 sau sync đầu tiên")
	assert.Equal(t, localDeck, *got, "B1: phải giữ đúng deck local")
}

// Kịch bản ngược lại, cũng bắt buộc đúng: peer CỐ TÌNH gỡ liên kết deck
// (`deck_id = NULL`) với mốc mới hơn ⇒ merge phải ghi NULL.
//
// Nếu test này FAIL vì `deck_id` bị giữ lại thì `UpsertStage` đã đánh đổi sai:
// "incoming KHÔNG mang thông tin" (nil) khác "incoming nói `deck_id IS NULL`".
func Test_B1_merge_writes_null_deck_id_when_peer_stage_really_has_no_deck(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()

	localDeck := seedDeck(t, local, "HSK1", "d-same", tsOld)
	localPath := seedPath(t, local, "zh", "p-same", tsOld)
	seedStageWithDeck(t, local, localPath, &localDeck, "s1", "st-same", tsOld)

	peerDeck := seedDeck(t, peer, "HSK1", "d-same", tsPeer)
	peerPath := seedPath(t, peer, "zh", "p-same", tsPeer)
	_ = peerDeck // deck vẫn tồn tại ở peer, chỉ stage KHÔNG gắn vào nó nữa
	seedStageWithDeck(t, peer, peerPath, nil, "s1", "st-same", tsPeer)
	require.NoError(t, peer.Exec("UPDATE roadmap_stages SET status = 'done' WHERE slug = 's1'").Error)

	svc := newService(t, local, peer)
	_, err := svc.Sync(ctx)
	require.NoError(t, err)

	assert.Nil(t, stageDeckID(t, local, "st-same"),
		"peer đã gỡ deck khỏi stage với mốc mới hơn ⇒ phải ghi NULL, không giữ cứng")
}

// B1 nhánh phòng thủ: `DeckGUID == nil` (incoming KHÔNG mang thông tin deck)
// thì `UpsertStage` phải KHÔNG đụng cột. Gọi thẳng repository để phủ được
// cả 3 trạng thái của con trỏ — loader hiện luôn gửi non-nil nên merge test
// không với tới nhánh này.
func Test_B1_UpsertStage_nil_deck_guid_keeps_local_deck_id(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()
	_ = peer

	deckID := seedDeck(t, local, "HSK1", "d-1", tsOld)
	pathID := seedPath(t, local, "zh", "p-1", tsOld)
	seedStageWithDeck(t, local, pathID, &deckID, "s1", "st-1", tsOld)

	repo := syncinfra.NewRepository(local)
	uow := syncinfra.NewUnitOfWork(local)

	err := uow.Do(ctx, func(tx syncapp.Tx) error {
		// incoming KHÔNG mang thông tin deck.
		_, _, err := repo.UpsertStage(ctx, tx, syncapp.StageRow{
			ID: 1, GUID: "st-1", PathGUID: "p-1", Slug: "s1", Title: "S-s1",
			Position: 1, Status: "done", DeckGUID: nil,
			Terrain: "meadow", Direction: "up",
			CreatedAt: tsOld, UpdatedAt: tsPeer,
		}, true)
		return err
	})
	require.NoError(t, err)
	got := stageDeckID(t, local, "st-1")
	require.NotNil(t, got, "incoming không mang deck ⇒ giữ nguyên deck_id local")
	assert.Equal(t, deckID, *got)
}

// ═══════════════════════════════════════════════════════════════════════════
// B2 — hồi sinh tombstone bị chính lần merge ghi đè ⇒ oscillation vĩnh viễn
// ═══════════════════════════════════════════════════════════════════════════

// tsMid nằm GIỮA `tsOld` (mốc tombstone local) và `tsPeer` (mốc thẻ sống của
// peer). Bắt buộc phải có mốc này: nếu tombstone của peer cũng bằng `tsOld`
// thì `Decide` ra `ActionKeep` (cùng mốc + cả hai vế đều đã xoá ⇒ không ai
// thắng) và bug B2 không lộ ra — test sẽ xanh nhầm.
const tsMid = "2026-09-15T00:00:00Z"

// seedB2Conflict dựng đúng mâu thuẫn Oracle mô tả: local có `c-zzz`
// tombstone @ `tsOld`, snapshot peer chứa `c-aaa` SỐNG @ `tsPeer` VÀ `c-zzz`
// tombstone @ `tsMid` (MỚI HƠN bản local) — cùng 1 `front`.
func seedB2Conflict(t *testing.T, local, peer *gorm.DB) int64 {
	t.Helper()
	localDeck := seedDeck(t, local, "HSK1", "d-same", tsOld)
	localCard := seedCard(t, local, localDeck, "你好", "c-zzz", tsOld)
	require.NoError(t, local.Exec("UPDATE cards SET deleted = 1 WHERE id = ?", localCard).Error)

	peerDeck := seedDeck(t, peer, "HSK1", "d-same", tsPeer)
	seedCard(t, peer, peerDeck, "你好", "c-aaa", tsPeer)
	// Phải insert thẳng `deleted = 1`: insert 2 thẻ cùng front rồi mới xoá 1
	// sẽ đụng `ux_cards_deck_front` (`WHERE deleted = 0`) ngay lúc insert.
	require.NoError(t, peer.Raw(`INSERT INTO cards
		(deck_id, front, back, state, due_at, created_at, guid, updated_at, deleted)
		VALUES (?, '你好', 'b-你好', 'new', ?, ?, 'c-zzz', ?, 1)`,
		peerDeck, tsOld, tsOld, tsMid).Error)
	return localCard
}

func cardState(t *testing.T, db *gorm.DB, id int64) (guid string, deleted int) {
	t.Helper()
	require.NoError(t, db.Raw("SELECT guid, deleted FROM cards WHERE id = ?", id).
		Row().Scan(&guid, &deleted))
	return
}

// Trước fix: local kết thúc CHỈ CÒN `c-zzz deleted=1` — thẻ sống của peer
// biến mất, trong khi conflict log ghi `recreate-adopted-tombstone` nên user
// tưởng đã vào. Lần sync sau lặp lại yệt.
func Test_B2_revived_card_survives_peer_tombstone_in_same_snapshot(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()
	localCard := seedB2Conflict(t, local, peer)

	svc := newService(t, local, peer)
	res, err := svc.Sync(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, res.Conflicts,
		"phải log xung đột để user thấy peer có tombstone mâu thuẫn với thẻ sống")

	guid, deleted := cardState(t, local, localCard)
	assert.Equal(t, "c-aaa", guid, "B2: phải nhận guid của thẻ sống từ peer")
	assert.Equal(t, 0, deleted,
		"B2: tombstone của chính lần merge đã xoá ngược thẻ vừa hồi sinh ⇒ oscillation")
	assert.Equal(t, 1, countRows(t, local, "cards"), "không được nhân đôi thẻ")
}

// B2 phần 2: lần sync thứ 2 phải ỔN ĐỊNH. Trước fix, mỗi lần sync lại hồi
// sinh rồi xoá ⇒ dữ liệu nhấp nháy vĩnh viễn.
func Test_B2_second_merge_does_not_oscillate(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()
	localCard := seedB2Conflict(t, local, peer)

	svc := newService(t, local, peer)
	_, err := svc.Sync(ctx)
	require.NoError(t, err)
	g1, d1 := cardState(t, local, localCard)

	// Peer giữ nguyên snapshot mâu thuẫn (không "trả lại" dữ liệu).
	_, err = svc.Sync(ctx)
	require.NoError(t, err)
	g2, d2 := cardState(t, local, localCard)

	assert.Equal(t, g1, g2, "B2: lần sync 2 phải cho cùng kết quả — không oscillation")
	assert.Equal(t, d1, d2)
	assert.Equal(t, 0, d2, "B2: thẻ sống phải sống mãi sau mọi lần sync")
	assert.Equal(t, 1, countRows(t, local, "cards"), "không được nhân đôi qua nhiều lần sync")
}

// B2 áp dụng CHUNG cho mọi bảng dùng luật "đọc `local` 1 lần": thêm 1 bảng
// roadmap cũng phải cập nhật map sau khi ghi. Test ở đây dùng `roadmap_paths`
// (bảng đơn giản nhất) để chứng minh pattern đã mở rộng — nếu ai đó gỡ
// `local[...] = in` ở `mergePaths` thì test này vẫn xanh, nên nó chỉ là chốt
// an toàn, không phải bằng chứng B2. Bằng chứng B2 là 2 test phía trên.
func Test_B2_local_map_updated_after_write_does_not_reinsert_same_guid(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()

	seedDeck(t, local, "HSK1-local", "d-1", tsOld)
	// Peer gửi CÙNG 1 path 2 lần trong snapshot (guid trùng) — trường hợp mà
	// map `local` cũ khiến lần 2 đi nhánh insert lại.
	peerPath := seedPath(t, peer, "zh", "p-1", tsPeer)
	seedStage(t, peer, peerPath, "s1", "st-1", tsPeer)
	require.NoError(t, peer.Exec(
		"UPDATE roadmap_paths SET overview = 'v2' WHERE id = ?", peerPath).Error)

	svc := newService(t, local, peer)
	_, err := svc.Sync(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, countRows(t, local, "roadmap_paths"),
		"cùng 1 guid không được tạo 2 row")
}

// ═══════════════════════════════════════════════════════════════════════════
// H3 — đọc pool BÊN TRONG transaction merge
// ═══════════════════════════════════════════════════════════════════════════

// Trong Postgres, đọc qua connection khác KHÔNG thấy row transaction vừa ghi.
// Hàm nào đọc bằng `r.read()` (pool) mà được gọi từ trong `uow.Do` sẽ trả kết
// quả sai — và sai thì IM LẶNG.
//
// Mấu chốt của test: phải GHI QUA `tx`, không dùng `local.Exec`. `local.Exec`
// đi qua pool, commit ngay, nên pool vẫn thấy và test xanh nhầm. Vì vậy test
// dùng chính `repo.UpsertCard`/`AppendReview`/`AppendNote` — đúng đường merge
// đi, và đúng đường đó là nơi lỗi xảy ra.
//
// Trước fix, `TombstoneCard` dùng `r.read()` ⇒ `ok=false` dù row vừa insert
// nằm ngay đó ⇒ merge KHÔNG hồi sinh tombstone, tạo row mới lệch guid.
func Test_H3_TombstoneCard_reads_inside_merge_transaction(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()
	_ = peer

	repo := syncinfra.NewRepository(local)
	uow := syncinfra.NewUnitOfWork(local)
	deckID := seedDeck(t, local, "HSK1", "d-1", tsOld)

	var gotID int64
	var gotGUID string
	var gotOK bool
	err := uow.Do(ctx, func(tx syncapp.Tx) error {
		// Ghi QUA tx: đây là dữ liệu chỉ tồn tại trong transaction này.
		written, _, err := repo.UpsertCard(ctx, tx, syncapp.CardRow{
			GUID: "c-tomb", DeckGUID: "d-1", DeckID: &deckID, Front: "你好",
			Back: "b", State: "new", CreatedAt: tsOld, UpdatedAt: tsOld, Deleted: 1,
		}, false)
		if err != nil {
			return err
		}
		id, guid, ok, err := repo.TombstoneCard(ctx, tx, deckID, "你好")
		if err != nil {
			return err
		}
		gotID, gotGUID, gotOK = id, guid, ok
		assert.Equal(t, written, gotID, "phải trả đúng id vừa ghi trong transaction")
		return nil
	})
	require.NoError(t, err)
	assert.True(t, gotOK, "H3: đọc trong transaction phải thấy row vừa ghi trong chính nó")
	assert.Equal(t, "c-tomb", gotGUID)
	assert.NotZero(t, gotID)
}

// Cùng lập luận cho `LiveCardGUIDByFront` — nhánh "2 máy cùng tạo 1 thẻ tay".
func Test_H3_LiveCardGUIDByFront_reads_inside_transaction(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()
	_ = peer

	repo := syncinfra.NewRepository(local)
	uow := syncinfra.NewUnitOfWork(local)
	deckID := seedDeck(t, local, "HSK1", "d-1", tsOld)

	var gotGUID string
	var gotOK bool
	err := uow.Do(ctx, func(tx syncapp.Tx) error {
		if _, _, err := repo.UpsertCard(ctx, tx, syncapp.CardRow{
			GUID: "c-live", DeckGUID: "d-1", DeckID: &deckID, Front: "再见",
			Back: "b", State: "new", CreatedAt: tsOld, UpdatedAt: tsOld,
		}, false); err != nil {
			return err
		}
		_, guid, ok, err := repo.LiveCardGUIDByFront(ctx, tx, deckID, "再见")
		if err != nil {
			return err
		}
		gotGUID, gotOK = guid, ok
		return nil
	})
	require.NoError(t, err)
	assert.True(t, gotOK, "H3: phải thấy thẻ vừa insert trong cùng transaction")
	assert.Equal(t, "c-live", gotGUID)
}

// `ReviewGUIDs`/`NoteGUIDs` cũng từng đọc pool trong khi union append-only:
// phải thấy review/note vừa append trong cùng lô merge, nếu không thì 1
// transaction ghi 2 dòng cùng guid sẽ đụng UNIQUE và hủy cả merge.
func Test_H3_ReviewGUIDs_and_NoteGUIDs_read_inside_transaction(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()
	_ = peer

	repo := syncinfra.NewRepository(local)
	uow := syncinfra.NewUnitOfWork(local)
	deckID := seedDeck(t, local, "HSK1", "d-1", tsOld)
	cardID := seedCard(t, local, deckID, "你好", "c-1", tsOld)

	var sawReview, sawNote bool
	err := uow.Do(ctx, func(tx syncapp.Tx) error {
		if _, err := repo.AppendReview(ctx, tx, syncapp.ReviewRow{
			GUID: "r-just-added", CardID: &cardID, Grade: 3,
			ReviewedAt: tsOld, NextDueAt: tsOld,
		}); err != nil {
			return err
		}
		if _, err := repo.AppendNote(ctx, tx, syncapp.NoteRow{
			GUID: "n-just-added", CardID: &cardID, Text: "SHADOW|note", CreatedAt: tsOld,
		}); err != nil {
			return err
		}
		reviews, err := repo.ReviewGUIDs(ctx, tx)
		if err != nil {
			return err
		}
		notes, err := repo.NoteGUIDs(ctx, tx)
		if err != nil {
			return err
		}
		sawReview = reviews["r-just-added"]
		sawNote = notes["n-just-added"]
		return nil
	})
	require.NoError(t, err)
	assert.True(t, sawReview, "H3: ReviewGUIDs phải thấy review vừa append trong transaction")
	assert.True(t, sawNote, "H3: NoteGUIDs phải thấy note vừa append trong transaction")
}

// ═══════════════════════════════════════════════════════════════════════════
// H4 — `txOf` rơi về pool im lặng khi tx sai kiểu
// ═══════════════════════════════════════════════════════════════════════════

// Trước fix `txOf` trả pool khi không nhận ra handle ⇒ **cả merge ghi ra NGOÀI
// transaction**, rollback không được mà test vẫn xanh vì "dữ liệu vẫn tới nơi".
func Test_H4_sync_repository_rejects_foreign_tx_and_writes_nothing(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()
	_ = peer

	repo := syncinfra.NewRepository(local)
	_, err := repo.UpsertDeck(ctx, wrongTx{ctx}, syncapp.DeckRow{
		Name: "SHOULD-NOT-BE-WRITTEN", Lang: "zh",
		CreatedAt: tsOld, GUID: "d-ghost", UpdatedAt: tsOld,
	}, false)
	require.Error(t, err, "tx sai kiểu phải báo LỖI, không rơi về pool")
	assert.Contains(t, err.Error(), "tx handle sai context")
	assert.Equal(t, 0, countRows(t, local, "decks"),
		"H4: phải KHÔNG ghi gì cả — rơi về pool là ghi ngoài transaction")
}

func Test_H4_content_repository_rejects_foreign_tx(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()
	_ = peer

	repo := contentinfra.NewRepository(local)
	_, err := repo.InsertDict(ctx, wrongTx{ctx}, &contentapp.ZHEntry{
		Hanzi: "猫", Pinyin: "mao1", Nghia: "mèo",
	})
	require.Error(t, err, "tx sai kiểu phải báo LỖI, không rơi về pool")
	assert.Contains(t, err.Error(), "tx handle sai context")
	assert.Equal(t, 0, countRows(t, local, "dict"), "phải KHÔNG ghi gì cả")
}

func Test_H4_practice_repository_rejects_foreign_tx(t *testing.T) {
	local, peer := twoMachines(t)
	ctx := context.Background()
	_ = peer

	repo := practiceinfra.NewRepository(local)
	err := repo.AppendNote(ctx, wrongTx{ctx}, &practiceapp.Note{
		Text: "SHOULD-NOT-BE-WRITTEN", CreatedAt: tsOld, GUID: "n-ghost",
	})
	require.Error(t, err, "tx sai kiểu phải báo LỖI, không rơi về pool")
	assert.Contains(t, err.Error(), "tx handle sai context")
	assert.Equal(t, 0, countRows(t, local, "notes"), "phải KHÔNG ghi gì cả")
}
