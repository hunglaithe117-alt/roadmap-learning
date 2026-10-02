package sync

// Test cho remediation cổng Oracle M3 — phần ở tầng application.
//
// B1 (nửa còn lại): Oracle nêu side-effect "merge.go bỏ `deck_guid` khỏi
// `stageValues` ⇒ `differs()` luôn true ⇒ luật '2 bản giống hệt → keep' không
// bao giờ chạy cho stage có deck". `differs` nằm ở `domain/sync` (đã có sẵn từ
// M1) nên test ở đây chỉ cần chứng minh `stageValues` sản ra 2 map BẰNG NHAU
// khi 2 bản giống hệt — kể cả khi `deck_guid` rỗng ở cả hai bên.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "langapp/internal/domain/sync"
)

func strptr(s string) *string { return &s }

// Test_B1_stageValues_match_when_both_copies_identical là phần "differs() trả
// false khi 2 bản giống hệt" của Oracle: 2 bản stage giống nhau (cùng gắn
// deck) phải cho `Values` bằng nhau.
func Test_B1_stageValues_match_when_both_copies_identical(t *testing.T) {
	st := StageRow{
		GUID: "st-1", PathGUID: "p-1", Slug: "s1", Title: "S-s1",
		Position: 1, Status: "done", Terrain: "meadow", Direction: "up",
		DeckGUID:  strptr("d-1"),
		CreatedAt: tsOld, UpdatedAt: tsOld,
	}
	local := stageValues(st, "d-1")
	incoming := stageValues(st, "")

	assert.Equal(t, local, incoming,
		"B1: 2 bản giống hệt phải cho Values bằng nhau")
	assert.Equal(t, "d-1", incoming["deck_guid"],
		"deck_guid phải có mặt kể cả khi incoming không mang (nil) — thiếu khoá làm differs() true")
}

// Kịch bản B1 gốc: incoming KHÔNG mang `deck_guid` (nil, snapshot cũ) còn local
// CÓ. `differs` phải trả false — vì merge sẽ không đụng `deck_id` nên coi như
// không đổi. Nếu `differs` true thì merge chạy vô ích và, tệ hơn, bất kỳ bản sửa
// nào khác trên stage cũng bị ghi đè bởi 1 UPDATE cột rỗng.
func Test_B1_stageValues_degrade_gracefully_when_incoming_lacks_deck(t *testing.T) {
	local := stageValues(StageRow{DeckGUID: strptr("d-1")}, "d-1")
	incoming := stageValues(StageRow{DeckGUID: nil}, "d-1")
	assert.Equal(t, local, incoming,
		"B1: incoming không mang deck ⇒ coi như giữ nguyên deck local")
}

// 2 bản THỰC SỰ khác nhau ở deck vẫn phải ra khác nhau — nếu không thì merge
// sẽ không bao giờ áp thay đổi gỡ deck.
func Test_B1_stageValues_differ_when_deck_really_changed(t *testing.T) {
	local := stageValues(StageRow{DeckGUID: strptr("d-1")}, "d-1")
	incoming := stageValues(StageRow{DeckGUID: strptr("d-2")}, "d-1")
	assert.NotEqual(t, local, incoming,
		"peer đổi deck thật thì phải khác nhau, nếu không merge không bao giờ ghi")
}

// `completed_at` NULL ở cả hai bên phải cho map BẰNG NHAU. Loader bỏ khoá của
// cột NULL (thay vì ghi `""`), nên nếu tầng application vẫn nhét `completed_at`
// vào map thì 2 bản giống nhau sẽ "khác nhau" — đúng cái bẫy khiến luật
// "2 bản giống hệt → keep" không bao giờ chạy.
func Test_B1_stageValues_omit_nil_completed_at(t *testing.T) {
	local := stageValues(StageRow{CompletedAt: nil, DeckGUID: strptr("d-1")}, "d-1")
	incoming := stageValues(StageRow{CompletedAt: nil, DeckGUID: strptr("d-1")}, "d-1")
	assert.Equal(t, local, incoming)
	_, present := local["completed_at"]
	assert.False(t, present, "completed_at NULL không được nhét vào Values")
}

// B2: sau khi hồi sinh tombstone, map `local` phải phản ánh DB.
//
// Kịch bản đúng như Oracle: local chỉ có tombstone `c-zzz`; CÙNG 1 snapshot
// peer chứa `c-aaa` SỐNG (mốc mới hơn) VÀ `c-zzz` tombstone (mốc cũ) — cùng
// `front`. Nếu sau khi hồi sinh map `local` không được cập nhật, incoming
// `c-zzz` vẫn khớp bản cũ trong map ⇒ `Decide` ra UPDATE ⇒ ghi
// `guid='c-zzz', deleted=1` đè lên chính thẻ vừa hồi sinh.
func Test_B2_merge_is_stable_when_peer_snapshot_contains_both_live_card_and_its_tombstone(t *testing.T) {
	// Thứ tự thẻ sống TRƯỚC tombstone là biến quan trọng: thứ tự row trong
	// snapshot là tuỳ peer (loader `SELECT` không `ORDER BY`), và chỉ khi thẻ
	// sống được xử lý trước thì tombstone cũ mới kịp "đụng lại" bản đã hồi
	// sinh. Test cố tình chạy CẢ HAI thứ tự — merge phải đúng bất kể.
	for _, tc := range []struct {
		name  string
		order []domain.Row
	}{
		{"thẻ sống trước tombstone", nil},
		{"tombstone trước thẻ sống", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			liveRow := snapRow(domain.TableCards, "c-aaa", "d1", tsPeer, 0, map[string]string{
				"front": "你好", "back": "xin chào", "state": "new", "due_at": tsPeer,
			})
			// Mốc `tsMid` MỚI HƠN bản tombstone local: nếu cũng bằng `tsOld`
			// thì `Decide` ra Keep (cùng mốc + cả hai vế đều đã xoá) và bug B2
			// không lộ ra — test sẽ xanh nhầm.
			tombRow := snapRow(domain.TableCards, "c-zzz", "d1", tsMid, 1, map[string]string{
				"front": "你好", "back": "b-你好", "state": "new", "due_at": tsOld,
			})
			rows := []domain.Row{liveRow, tombRow}
			if tc.name == "tombstone trước thẻ sống" {
				rows = []domain.Row{tombRow, liveRow}
			}

			ctx := context.Background()
			repo := newFakeRepo()
			_, err := repo.UpsertDeck(ctx, nil,
				DeckRow{GUID: "d1", Name: "HSK1", Lang: "zh", CreatedAt: tsOld, UpdatedAt: tsOld}, false)
			require.NoError(t, err)
			seedLocalCard(repo, "c-zzz", "d1", "你好", 1, tsOld)
			tombID := repo.cards["c-zzz"].id

			snap := domain.PeerSnapshot{
				SchemaVersion: 4,
				Rows: append([]domain.Row{
					snapRow(domain.TableDecks, "d1", "", tsOld, 0,
						map[string]string{"name": "HSK1", "lang": "zh"}),
				}, rows...),
			}
			svc, _ := newService(repo, snap)

			_, err = svc.Merge(ctx, snap)
			require.NoError(t, err)
			cell, ok := repo.cards["c-aaa"]
			require.True(t, ok, "B2: phải hồi sinh thẻ sống mang guid của peer")
			assert.Equal(t, tombID, cell.id, "hồi sinh phải giữ id để reviews.card_id còn trỏ đúng")
			assert.Equal(t, 0, cell.payload.(CardRow).Deleted, "B2: thẻ phải SỐNG sau lần 1")

			// Lần 2: cùng snapshot, peer không "trả lại" dữ liệu. Trước fix,
			// lần này xoá ngược chính thẻ vừa hồi sinh — oscillation vĩnh viễn.
			_, err = svc.Merge(ctx, snap)
			require.NoError(t, err)
			cell2, ok := repo.cards["c-aaa"]
			require.True(t, ok, "B2: lần 2 phải còn thẻ sống — không oscillation")
			assert.Equal(t, 0, cell2.payload.(CardRow).Deleted)
			assert.Equal(t, cell.payload.(CardRow).UpdatedAt, cell2.payload.(CardRow).UpdatedAt,
				"B2: lần 2 phải cho cùng kết quả — không nhấp nháy dữ liệu")
			assert.Len(t, repo.cards, 1, "B2: không được nhân đôi thẻ")
		})
	}
}
