package sync

import (
	"fmt"
	"time"
)

// Action defines the merge strategy to execute for a row.
type Action string

const (
	// ActionInsert inserts an incoming row when absent locally.
	ActionInsert Action = "insert"
	// ActionUpdate updates a local row when the incoming row wins.
	ActionUpdate Action = "update"
	// ActionKeep retains the local row version.
	ActionKeep Action = "keep"
	// ActionAppend appends a record in append-only tables.
	ActionAppend Action = "append"
	// ActionSkip ignores an incoming record whose parent is missing.
	ActionSkip Action = "skip"
)

// MergeDecision represents the resolved merge action for a row.
type MergeDecision struct {
	Action   Action
	Winner   string
	Conflict *Conflict
	Reason   string
}

// Merge encapsulates local and incoming row data for conflict resolution.
type Merge struct {
	Local    *Row
	Incoming Row
	LastSync string
	Now      string
}

// Decide determines the Last-Write-Wins merge decision between local and incoming rows.
func Decide(m Merge) MergeDecision {
	in := m.Incoming
	incTS := in.EffectiveTS()
	del := in.Deleted
	if in.IsTombstone() {
		del = 1
	}

	if m.Local == nil {
		return MergeDecision{Action: ActionInsert, Reason: "missing local row"}
	}
	local := *m.Local
	localTS := local.EffectiveTS()

	if incTS == localTS && del == local.Deleted && !differs(local, in) {
		return MergeDecision{Action: ActionKeep, Winner: WinnerLocal, Reason: "identical payload"}
	}

	incWins := incTS > localTS
	tieTombstoneWins := incTS == localTS && del == 1 && local.Deleted == 0
	if tieTombstoneWins {
		incWins = true
	}

	decision := MergeDecision{Action: ActionKeep, Winner: WinnerLocal, Reason: "local retained"}
	if incWins {
		decision = MergeDecision{
			Action: ActionUpdate,
			Winner: WinnerIncoming,
			Reason: "incoming newer",
		}
		if tieTombstoneWins {
			decision.Reason = "equal timestamp tombstone wins"
		}
	}

	if m.LastSync != "" && incTS > m.LastSync && localTS > m.LastSync {
		detail := fmt.Sprintf("both-changed local_ts=%s incoming_ts=%s deleted=%d->%d",
			localTS, incTS, local.Deleted, del)
		decision.Conflict = &Conflict{
			Table: in.Table, GUID: in.GUID, Winner: decision.Winner, Detail: detail, At: m.Now,
		}
	}
	return decision
}

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

// DecideAppend resolves insertion for append-only tables deduplicated by GUID.
func DecideAppend(in Row, knownLocal map[string]bool) MergeDecision {
	if in.GUID == "" {
		return MergeDecision{Action: ActionSkip, Reason: "missing GUID"}
	}
	if knownLocal[in.GUID] {
		return MergeDecision{Action: ActionSkip, Reason: "existing GUID"}
	}
	return MergeDecision{Action: ActionAppend, Reason: "new GUID"}
}

// DetectClockSkew detects potential time skew between host and backup snapshot.
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

// VersionMismatchMessage returns an error string describing schema incompatibility.
func VersionMismatchMessage(localV, incomingV int) string {
	return fmt.Sprintf(
		"schema version không khớp: máy này v%d, backup v%d — xóa DB làm lại, không có cách merge an toàn",
		localV, incomingV)
}

// CheckVersion verifies schema version compatibility between local and incoming snapshots.
func CheckVersion(localV, incomingV int) error {
	if localV != incomingV {
		return fmt.Errorf("%s", VersionMismatchMessage(localV, incomingV))
	}
	return nil
}

