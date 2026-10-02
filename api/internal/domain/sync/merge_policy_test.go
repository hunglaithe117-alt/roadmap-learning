package sync

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func row(ts string, deleted int, values map[string]string) *Row {
	return &Row{
		Table: TableDecks, GUID: "g1", UpdatedAt: ts, Deleted: deleted, Values: values,
	}
}

func Test_decide_inserts_when_local_missing(t *testing.T) {
	got := Decide(Merge{Incoming: Row{Table: TableDecks, GUID: "g1", UpdatedAt: "2026-01-01T00:00:00Z"}})
	assert.Equal(t, ActionInsert, got.Action)
	assert.Empty(t, got.Conflict, "lần chạm đầu tiên không có xung đột để log")
}

func Test_decide_keeps_when_both_rows_identical(t *testing.T) {
	values := map[string]string{"name": "Hanzi"}
	got := Decide(Merge{
		Local:    row("2026-01-01T00:00:00Z", 0, values),
		Incoming: Row{Table: TableDecks, GUID: "g1", UpdatedAt: "2026-01-01T00:00:00Z", Values: values},
		LastSync: "2025-12-01T00:00:00Z", Now: "2026-01-02T00:00:00Z",
	})
	assert.Equal(t, ActionKeep, got.Action)
	assert.Nil(t, got.Conflict, "2 bản giống hệt thì không phải xung đột")
}

func Test_decide_incoming_wins_when_newer(t *testing.T) {
	got := Decide(Merge{
		Local:    row("2026-01-01T00:00:00Z", 0, map[string]string{"name": "cũ"}),
		Incoming: Row{Table: TableDecks, GUID: "g1", UpdatedAt: "2026-01-03T00:00:00Z", Values: map[string]string{"name": "mới"}},
	})
	assert.Equal(t, ActionUpdate, got.Action)
	assert.Equal(t, WinnerIncoming, got.Winner)
}

func Test_decide_local_wins_when_newer(t *testing.T) {
	got := Decide(Merge{
		Local:    row("2026-01-05T00:00:00Z", 0, map[string]string{"name": "mới"}),
		Incoming: Row{Table: TableDecks, GUID: "g1", UpdatedAt: "2026-01-03T00:00:00Z", Values: map[string]string{"name": "cũ"}},
	})
	assert.Equal(t, ActionKeep, got.Action)
	assert.Equal(t, WinnerLocal, got.Winner)
	assert.Nil(t, got.Conflict, "chỉ 1 máy thay đổi thì không phải xung đột")
}

func Test_decide_tombstone_wins_on_equal_timestamp(t *testing.T) {
	// Cùng mốc, peer đã xóa mềm còn local thì chưa -> xóa phải thắng,
	// nếu không tombstone bị hồi sinh mỗi lần sync.
	got := Decide(Merge{
		Local:    row("2026-01-01T00:00:00Z", 0, map[string]string{"name": "x"}),
		Incoming: Row{Table: TableDecks, GUID: "g1", UpdatedAt: "2026-01-01T00:00:00Z", Deleted: 1},
	})
	assert.Equal(t, ActionUpdate, got.Action, "tombstone cùng mốc phải thắng")
	assert.Contains(t, got.Reason, "tombstone")
}

func Test_decide_stale_tombstone_loses_to_live_local(t *testing.T) {
	// Local đã sửa sau khi peer xóa -> bản mới hơn thắng, tombstone cũ bị bác.
	got := Decide(Merge{
		Local:    row("2026-01-10T00:00:00Z", 0, map[string]string{"name": "sửa lại"}),
		Incoming: Row{Table: TableDecks, GUID: "g1", UpdatedAt: "2026-01-01T00:00:00Z", Deleted: 1},
	})
	assert.Equal(t, ActionKeep, got.Action, "tombstone cũ không được hồi sinh bản đã sửa")
}

func Test_decide_logs_conflict_only_when_both_changed_after_last_sync(t *testing.T) {
	values := map[string]string{"name": "a"}
	got := Decide(Merge{
		Local:    row("2026-01-05T00:00:00Z", 0, values),
		Incoming: Row{Table: TableDecks, GUID: "g1", UpdatedAt: "2026-01-06T00:00:00Z", Values: map[string]string{"name": "b"}},
		LastSync: "2026-01-02T00:00:00Z", Now: "2026-01-07T00:00:00Z",
	})
	require.NotNil(t, got.Conflict, "cả 2 vế đổi sau lastSync = xung đột thật")
	assert.Equal(t, WinnerIncoming, got.Conflict.Winner)
	assert.Equal(t, TableDecks, got.Conflict.Table)
	assert.Contains(t, got.Conflict.Detail, "both-changed")
	assert.Equal(t, "2026-01-07T00:00:00Z", got.Conflict.At)
}

func Test_decide_skips_conflict_log_on_first_contact(t *testing.T) {
	got := Decide(Merge{
		Local:    row("2026-01-05T00:00:00Z", 0, map[string]string{"name": "a"}),
		Incoming: Row{Table: TableDecks, GUID: "g1", UpdatedAt: "2026-01-06T00:00:00Z", Values: map[string]string{"name": "b"}},
		LastSync: "", // chưa sync lần nào
		Now:      "2026-01-07T00:00:00Z",
	})
	require.Nil(t, got.Conflict, "lastSync rỗng -> first contact, không spam log conflict")
}

func Test_decide_does_not_log_conflict_when_only_one_side_changed(t *testing.T) {
	got := Decide(Merge{
		Local:    row("2026-01-01T00:00:00Z", 0, map[string]string{"name": "a"}),
		Incoming: Row{Table: TableDecks, GUID: "g1", UpdatedAt: "2026-01-06T00:00:00Z", Values: map[string]string{"name": "b"}},
		LastSync: "2026-01-02T00:00:00Z", Now: "2026-01-07T00:00:00Z",
	})
	assert.Nil(t, got.Conflict, "local đứng yên từ trước lastSync thì không phải xung đột")
}

func Test_decide_falls_back_to_created_at_when_updated_at_empty(t *testing.T) {
	local := &Row{Table: TableCards, GUID: "c1", CreatedAt: "2026-01-01T00:00:00Z", Deleted: 0}
	got := Decide(Merge{
		Local:    local,
		Incoming: Row{Table: TableCards, GUID: "c1", CreatedAt: "2026-02-01T00:00:00Z"},
	})
	assert.Equal(t, ActionUpdate, got.Action, "row chưa từng sửa thì so created_at")
}

// Cùng mốc thì LOCAL thắng, kể cả khi payload khác — quy tắc LWW là so
// timestamp, và timestamp đi trước. Ngoại lệ duy nhất ở mốc bằng là
// tombstone (xem Test_decide_tombstone_wins_on_equal_timestamp). Hệ quả:
// 2 máy sửa cùng giây thì bản local được giữ, có log conflict để user
// xử lý tay.
func Test_decide_equal_timestamp_keeps_local_even_when_payload_differs(t *testing.T) {
	got := Decide(Merge{
		Local:    row("2026-01-01T00:00:00Z", 0, map[string]string{"name": "a"}),
		Incoming: Row{Table: TableDecks, GUID: "g1", UpdatedAt: "2026-01-01T00:00:00Z", Values: map[string]string{"name": "KHÁC"}},
		LastSync: "2025-12-01T00:00:00Z", Now: "2026-01-02T00:00:00Z",
	})
	assert.Equal(t, ActionKeep, got.Action, "LWW quyết định bằng timestamp, không bằng payload")
	assert.Equal(t, WinnerLocal, got.Winner)
	require.NotNil(t, got.Conflict, "2 máy sửa cùng giây vẫn phải log để user tự xử lý")
	assert.Equal(t, WinnerLocal, got.Conflict.Winner)
}

func Test_decide_append_dedupes_by_guid(t *testing.T) {
	known := map[string]bool{"r1": true}
	assert.Equal(t, ActionAppend, DecideAppend(Row{Table: TableReviews, GUID: "r2"}, known).Action)
	assert.Equal(t, ActionSkip, DecideAppend(Row{Table: TableReviews, GUID: "r1"}, known).Action)
	assert.Equal(t, ActionSkip, DecideAppend(Row{Table: TableReviews, GUID: ""}, known).Action,
		"thiếu GUID thì không dedupe được, bỏ thay vì tạo bản trùng")
}

func Test_decide_append_does_not_mutate_known_set(t *testing.T) {
	known := map[string]bool{}
	row := Row{Table: TableReviews, GUID: "r1"}
	assert.Equal(t, ActionAppend, DecideAppend(row, known).Action)
	assert.Equal(t, ActionAppend, DecideAppend(row, known).Action,
		"policy không tự thêm vào tập — caller thêm sau khi insert thành công, nếu không 1 lần insert lỗi sẽ nuốt row")
	assert.Empty(t, known)
}

func Test_row_effective_ts_and_tombstone(t *testing.T) {
	assert.Equal(t, "2026-01-02T00:00:00Z", Row{UpdatedAt: "2026-01-02T00:00:00Z", CreatedAt: "2026-01-01T00:00:00Z"}.EffectiveTS())
	assert.Equal(t, "2026-01-01T00:00:00Z", Row{CreatedAt: "2026-01-01T00:00:00Z"}.EffectiveTS())
	assert.Equal(t, "", Row{}.EffectiveTS())
	assert.True(t, Row{Deleted: 1}.IsTombstone())
	assert.False(t, Row{Deleted: 0}.IsTombstone())
}

func Test_detect_clock_skew_flags_future_snapshot(t *testing.T) {
	got := DetectClockSkew("2026-01-02T00:00:00Z", "2026-01-01T00:00:00Z")
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "mới hơn giờ máy này")
}

func Test_detect_clock_skew_flags_stale_snapshot(t *testing.T) {
	got := DetectClockSkew("2026-01-01T00:00:00Z", "2026-01-05T00:00:00Z")
	require.Len(t, got, 1)
	assert.Contains(t, got[0], "cũ hơn giờ máy này")
}

func Test_detect_clock_skew_silent_within_threshold(t *testing.T) {
	assert.Empty(t, DetectClockSkew("2026-01-01T12:00:00Z", "2026-01-01T18:00:00Z"))
	assert.Empty(t, DetectClockSkew("", "2026-01-01T00:00:00Z"))
}

func Test_detect_clock_skew_ignores_unparsable_timestamps(t *testing.T) {
	assert.Empty(t, DetectClockSkew("không-parse", "2026-01-01T00:00:00Z"),
		"dữ liệu hỏng không phải lệch đồng hồ, báo sai còn gây hoang mang")
	assert.Empty(t, DetectClockSkew("2026-01-01T00:00:00Z", "không-parse"))
}

func Test_detect_clock_skew_threshold_is_one_day(t *testing.T) {
	now := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	justUnder := now.Add(-ClockSkewThreshold + time.Minute).Format(time.RFC3339)
	over := now.Add(-ClockSkewThreshold - time.Minute).Format(time.RFC3339)
	assert.Empty(t, DetectClockSkew(justUnder, now.Format(time.RFC3339)))
	assert.Len(t, DetectClockSkew(over, now.Format(time.RFC3339)), 1)
}

func Test_check_version_rejects_mismatch(t *testing.T) {
	require.NoError(t, CheckVersion(4, 4))
	err := CheckVersion(4, 3)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "v4")
	assert.Contains(t, err.Error(), "v3")
}

func Test_schema_version_is_four(t *testing.T) {
	assert.Equal(t, 4, SchemaVersion, "đồng bộ với schema v4 (api/schema.sql) + roadmap 5 bảng")
}
