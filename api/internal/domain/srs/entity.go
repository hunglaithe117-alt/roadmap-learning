// Package srs chứa bounded context spaced-repetition: entity `Deck`/`Card`,
// value object `Grade`, và policy lịch ôn `ScheduleNext` + `DueFilter`.
//
// Port từ api/srs.go v1 (giữ nguyên hằng số và đường tính — M3 gate: "logic
// 1:1 không đổi hành vi"). Package này thuần Go: không driver DB, không
// framework HTTP.
package srs

import (
	"strings"
	"time"
)

// Lang là ngôn ngữ của deck, frozen contract zh|en (xen khoá DB ở
// migrations/00001_init.sql).
const (
	LangZH = "zh"
	LangEN = "en"
)

// CardState là vòng đời ôn của 1 thẻ.
const (
	// StateNew = thẻ chưa ôn lần nào. `DueFilter` luôn trả về thẻ ở state này
	// kể cả khi due_at còn ở tương lai (CreateCard gán +24h).
	StateNew    = "new"
	StateReview = "review"
)

// Deck là bộ thẻ, gốc sở hữu card (FK cards.deck_id ON DELETE CASCADE).
type Deck struct {
	ID        int64
	Name      string
	Lang      string
	CreatedAt string
	// Sync columns: guid là natural key để peer merge, updated_at là mốc
	// LWW, deleted là tombstone 0|1. KHÔNG dùng gorm.DeletedAt (STACK-V2-PLAN
	// §4.4 — logic sync LWW đã xong phụ thuộc integer này).
	GUID      string
	UpdatedAt string
	Deleted   int
}

// Card là 1 thẻ ôn. Cột null của SQLite (tone/ipa/stress/audio_url) là *string
// ở đây để phân biệt NULL với chuỗi rỗng — v1 đọc bằng COALESCE nên mất
// khác biệt này; domain giữ nguyên để repository M2 quyết định.
type Card struct {
	ID     int64
	DeckID int64
	Front  string
	Back   string
	// Pinyin là pinyin có số thanh ("ni3 hao3"); `content` context dịch sang
	// dấu thanh qua MarkSyllable.
	Pinyin     string
	DueAt      time.Time
	Stability  float64
	Difficulty float64
	Reps       int
	Lapses     int
	State      string
	CreatedAt  time.Time
	Tone       *string
	IPA        *string
	Stress     *string
	AudioURL   *string
	GUID       string
	UpdatedAt  time.Time
	Deleted    int
}

// IsDueNow là điều kiện hẹn giờ mà DueFilter áp dụng: đã tới hạn HOẶC chưa
// từng ôn. Trả về false cho thẻ đã xóa mềm.
func (c Card) IsDueNow(now time.Time) bool {
	return !c.IsDeleted() && (c.State == StateNew || !c.DueAt.After(now))
}

// IsDeleted đọc tombstone 0|1 thành bool để policy không lặp `== 1`.
func (c Card) IsDeleted() bool { return c.Deleted != 0 }

// Review là 1 lần chấm điểm, lịch sử append-only (sync merge bằng cách union
// theo guid, không LWW).
type Review struct {
	ID         int64
	CardID     int64
	Grade      Grade
	ReviewedAt time.Time
	NextDueAt  time.Time
	GUID       string
}

// NormalizeLang map mọi biến thể về zh|en; trả false nếu không nhận ra.
func NormalizeLang(s string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "zh", "zh-cn", "zh_cn", "cn":
		return LangZH, true
	case "en", "en-us", "en_us":
		return LangEN, true
	default:
		return "", false
	}
}
