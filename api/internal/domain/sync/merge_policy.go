package sync

import (
	"fmt"
	"time"
)

// Action là quyết định của policy Merge: infrastructure thi hành theo đúng
// action này, không tự suy luận.
type Action string

const (
	// ActionInsert: row chưa có ở máy này -> tạo mới.
	ActionInsert Action = "insert"
	// ActionUpdate: incoming thắng -> ghi đè.
	ActionUpdate Action = "update"
	// ActionKeep: giữ bản local, bỏ incoming.
	ActionKeep Action = "keep"
	// ActionAppend: reviews/notes append-only, dedupe theo GUID.
	ActionAppend Action = "append"
	// ActionSkip: deck cha của incoming chưa map được -> bỏ row (cascade).
	ActionSkip Action = "skip"
)

// MergeDecision là kết quả quyết định cho 1 row.
type MergeDecision struct {
	Action Action
	// Winner là "incoming" khi Action = update, "local" khi Action = keep,
	// rỗng khi insert/skip.
	Winner string
	// Conflict là bản ghi conflict cần lưu (sync_conflicts), nil nếu không
	// phải xung đột thật.
	Conflict *Conflict
	// Reason là giải thích ngắn để debug, không dùng hiển thị.
	Reason string
}

// Merge là input của Decide: row local (đã có trong DB) và row incoming (từ
// peer), cùng mốc lastSync.
type Merge struct {
	Local    *Row
	Incoming Row
	LastSync string
	// Now là mốc ghi conflict; truyền vào để test được.
	Now string
}

// Decide áp quy tắc LWW cho 1 row có thể sửa (decks, cards, roadmap_*).
//
// Quy tắc (port từ mergeDecksCards v1):
//  1. local chưa có -> insert.
//  2. timestamp + payload giống hệt -> keep, không động vào DB.
//  3. incoming mới hơn (updated_at lớn hơn) -> update.
//  4. timestamp BẰNG NHAU mà incoming là tombstone còn local thì chưa
//     -> incoming thắng. Đây là điểm quan trọng: xóa mềm phải thắng khi hai
//     máy cùng mốc, nếu không tombstone có thể bị hồi sinh.
//  5. còn lại -> giữ local.
//  6. Ghi log conflict khi lastSync khác rỗng VÀ cả 2 vế đều thay đổi sau
//     lastSync (tức là thật sự xung đột, không phải 1 máy chỉ đọc).
func Decide(m Merge) MergeDecision {
	in := m.Incoming
	incTS := in.EffectiveTS()
	del := in.Deleted
	if in.IsTombstone() {
		del = 1
	}

	if m.Local == nil {
		return MergeDecision{Action: ActionInsert, Reason: "chưa có bản local"}
	}
	local := *m.Local
	localTS := local.EffectiveTS()

	if incTS == localTS && del == local.Deleted && !differs(local, in) {
		return MergeDecision{Action: ActionKeep, Winner: WinnerLocal, Reason: "2 bản giống hệt"}
	}

	incWins := incTS > localTS
	tieTombstoneWins := incTS == localTS && del == 1 && local.Deleted == 0
	if tieTombstoneWins {
		incWins = true
	}

	decision := MergeDecision{Action: ActionKeep, Winner: WinnerLocal, Reason: "local không bị thay"}
	if incWins {
		decision = MergeDecision{
			Action: ActionUpdate,
			Winner: WinnerIncoming,
			Reason: "incoming mới hơn",
		}
		if tieTombstoneWins {
			decision.Reason = "cùng mốc + incoming là tombstone -> xóa mềm thắng"
		}
	}

	// Log conflict khi CẢ HAI vế đều thay đổi sau lastSync, bất kể ai thắng:
	// đây là lần user cần biết để tự xử lý.
	if m.LastSync != "" && incTS > m.LastSync && localTS > m.LastSync {
		detail := fmt.Sprintf("both-changed local_ts=%s incoming_ts=%s deleted=%d->%d",
			localTS, incTS, local.Deleted, del)
		decision.Conflict = &Conflict{
			Table: in.Table, GUID: in.GUID, Winner: decision.Winner, Detail: detail, At: m.Now,
		}
	}
	return decision
}

// differs so payload có khác nhau không (bỏ qua cột timestamp đã so riêng).
func differs(local, incoming Row) bool {
	if len(local.Values) != len(incoming.Values) {
		return true
	}
	for k, v := range incoming.Values {
		if local.Values[k] != v {
			return true
		}
	}
	return false
}

// DecideAppend cho bảng append-only (reviews, notes): không LWW, chỉ union
// theo GUID. Trả về ActionAppend cho GUID mới, ActionSkip cho GUID đã có.
//
// knownLocal là tập GUID đã có ở máy này; hàm KHÔNG mutate nó — caller giữ
// tập và tự thêm sau khi insert thành công (nếu không, 1 lần insert lỗi sẽ
// làm mất row).
func DecideAppend(in Row, knownLocal map[string]bool) MergeDecision {
	if in.GUID == "" {
		return MergeDecision{Action: ActionSkip, Reason: "thiếu GUID thì không dedupe được, bỏ"}
	}
	if knownLocal[in.GUID] {
		return MergeDecision{Action: ActionSkip, Reason: "GUID đã có ở local"}
	}
	return MergeDecision{Action: ActionAppend, Reason: "GUID mới"}
}

// DetectClockSkew cảnh báo khi đồng hồ 2 máy lệch. Trả về nil khi ổn.
//
// 2 trường hợp nguy hiểm:
//   - maxIncomingTS > now: backup mới hơn giờ máy này -> máy này có đồng hồ
//     lùi, mọi row từ peer sẽ thắng LWW.
//   - now - maxIncomingTS > 24h: backup cũ hơn nhiều -> nghi máy peer lùi
//     đồng hồ, LWW sẽ bác bỏ thay đổi thật.
//
// Timestamp không parse được thì KHÔNG cảnh báo: dữ liệu hỏng không phải
// lệch đồng hồ, báo sai còn gây hoang mang.
func DetectClockSkew(maxIncomingTS, now string) []string {
	if maxIncomingTS == "" {
		return nil
	}
	nowT, err := time.Parse(time.RFC3339, now)
	if err != nil {
		return nil
	}
	maxT, err := time.Parse(time.RFC3339, maxIncomingTS)
	if err != nil {
		return nil
	}
	skew := nowT.Sub(maxT)
	if skew < 0 {
		return []string{fmt.Sprintf(
			"clock-skew: backup mới hơn giờ máy này %s (max updated_at=%s) — kiểm tra đồng hồ 2 máy",
			(-skew).Round(time.Second), maxIncomingTS)}
	}
	if skew > ClockSkewThreshold {
		return []string{fmt.Sprintf(
			"clock-skew: backup cũ hơn giờ máy này %s (max updated_at=%s) — LWW có thể sai nếu đồng hồ lệch",
			skew.Round(time.Second), maxIncomingTS)}
	}
	return nil
}

// VersionMismatchMessage là thông báo khi 2 máy lệch schema version.
func VersionMismatchMessage(localV, incomingV int) string {
	return fmt.Sprintf(
		"schema version không khớp: máy này v%d, backup v%d — xóa DB làm lại, không có cách merge an toàn",
		localV, incomingV)
}

// CheckVersion trả về lỗi (dạng thông điệp) khi version lệch.
func CheckVersion(localV, incomingV int) error {
	if localV != incomingV {
		return fmt.Errorf("%s", VersionMismatchMessage(localV, incomingV))
	}
	return nil
}
