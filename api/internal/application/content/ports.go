// Package content chứa tầng orchestration của bounded context nội dung học:
// tra từ điển Trung/Anh, import seed HSK, dấu thanh Pinyin, chunking câu,
// trọng âm, checklist THIEU và bài đọc graded.
//
// Lớp này KHÔNG import driver DB và KHÔNG import tầng hạ tầng
// (STACK-V2-PLAN §2). Mọi truy cập DB đi qua interface dưới đây, hiện thực
// nằm ở tầng hạ tầng (package `contentinfra`).
//
// Quy tắc cây "đừng đi vòng": quyết định LWW/tombstone nằm ở domain/sync,
// chấm thanh + chunk + trọng âm + THIEU nằm ở domain/content. Tầng này chỉ
// điều phối và validate input.
package content

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Tx là handle transaction do UnitOfWork tạo và đưa cho repository.
//
// Kiểu `any` cố ý: application không biết driver, còn infrastructure tự ép
// kiểu về *gorm.DB. Đổi sang interface có method sẽ buộc infrastructure phải
// implement marker method của package này — tức là hướng phụ thuộc đảo ngược.
type Tx = any

// UnitOfWork chạy 1 khối ghi trong 1 transaction; callback trả lỗi ⇒ rollback
// toàn bộ. ImportHSK dùng nó vì deck + 36 thẻ + dict phải là khối nguyên tử:
// crash giữa chừng để lại deck rỗng rồi lần import sau tưởng đã xong.
type UnitOfWork interface {
	Do(ctx context.Context, fn func(tx Tx) error) error
}

// Repository là toàn bộ truy cập DB của context content.
//
// Tra từ điển ĐI QUA HÀM SQL `dict_search` / `en_dict_search` đã có ở
// migrations/00002_fts.sql, KHÔNG viết lại truy vấn ở đây: hàm đó đã xử lý
// `websearch_to_tsquery` (chịu cú pháp người dùng gõ thẳng) + `ILIKE` lưới
// vớ CJK ngắn + khử ký tự wildcard `%_\` của input. Viết lại truy vấn ở tầng
// Go là chỗ hỏng F5 của M1 tái diễn.
//
// MỌI METHOD GHI nhận `Tx` làm tham số đầu — đó là cách duy nhất để
// repository biết ghi vào transaction nào. Method đọc không nhận.
type Repository interface {
	// SearchZH tra dict (chữ Hán) qua hàm SQL `dict_search`.
	SearchZH(ctx context.Context, q string, limit int) ([]ZHEntry, error)
	// SearchEN tra en_dict qua hàm SQL `en_dict_search`.
	SearchEN(ctx context.Context, q string, limit int) ([]ENEntry, error)
	// LookupEN tra CHÍNH XÁC 1 headword trong en_dict. Khoá là
	// `lower(term) = lower(?)` để dùng được index `idx_en_dict_term_lower` đã
	// tạo ở migration 00002. KHÔNG dùng `COLLATE NOCASE`: đó là cú pháp
	// SQLite, **lỗi cú pháp trong Postgres** (xem nợ #1 trong
	// .slim/deepwork/stack-v2.md — api/english.go:274 v1 vẫn còn lỗi này vì
	// app cũ chạy SQLite).
	LookupEN(ctx context.Context, term string) (ENEntry, bool, error)
	// DictHanziSet trả tập chữ Hán đã có trong dict — import HSK dùng để
	// không chèn lại trong cùng 1 lần (và để báo `dict_added` trung thực).
	//
	// Nhận `tx`: import HSK vừa chèn hàng chục dòng dict trong cùng khối, đọc
	// ngoài transaction sẽ không thấy chúng (chưa commit) và `dict_added` sai.
	DictHanziSet(ctx context.Context, tx Tx) (map[string]bool, error)
	// InsertDict chèn 1 dòng dict. Trả false nếu chữ đã có (ON CONFLICT /
	// check trước) — import là idempotent theo chữ Hán.
	InsertDict(ctx context.Context, tx Tx, e *ZHEntry) (bool, error)
	// CountDict đếm số dòng dict.
	CountDict(ctx context.Context) (int, error)

	// InsertEN chèn 1 dòng en_dict (seed từ điển tích hợp).
	InsertEN(ctx context.Context, tx Tx, e *ENEntry) (bool, error)
	// CountEN đếm số dòng en_dict.
	CountEN(ctx context.Context) (int, error)

	// NoteWriter cho prefix THIEU| (checklist 8 trục). Tách khỏi Repository
	// vì nó ghi vào bảng `notes` mà context `practice` cũng dùng với prefix
	// SHADOW|/ERR| — cùng bảng, khác chủ sở hữu theo prefix.
	InsertNote(ctx context.Context, tx Tx, n *Note) error
	// ListNotesByPrefix trả note có prefix, mới nhất trước.
	ListNotesByPrefix(ctx context.Context, prefix string, limit int) ([]Note, error)
}

// CardToneWriter là port sang bounded context `srs`: ghi thanh điệu đã chấm vào
// `cards.tone`. Cột đó thuộc bảng `cards` (srs sở hữu) nhưng quy tắc chấm
// thuộc `content` (bảng thang điệu) — đó là lý do `srs.Service.SetCardTone` ở
// M2 chỉ GHI kết quả, còn bước chấm để context này gọi.
type CardToneWriter interface {
	// SetCardTone ghi tone (nil = xoá) và trả cột tone sau khi ghi.
	SetCardTone(ctx context.Context, tx Tx, cardID int64, tone *string) (string, error)
}

// CardReader là port sang `srs` chỉ để kiểm tra thẻ tồn tại (import HSK và
// SetTone đều thao tác lên `cards`).
type CardReader interface {
	CardExists(ctx context.Context, id int64) (bool, error)
}

// DeckWriter là port sang `srs` cho import/seed: tạo deck và thẻ hàng loạt mà
// KHÔNG đi qua use case srs từng thẻ một (import HSK1 = 36 thẻ, mỗi thẻ 1
// transaction riêng là 36 fsync cho 1 thao tác logic).
type DeckWriter interface {
	// EnsureSeedDeck trả deck đang sống theo tên, tạo mới nếu chưa có.
	// Trả ErrNotFound khi tồn tại nhưng lang sai — caller báo 400.
	EnsureSeedDeck(ctx context.Context, tx Tx, name, lang, guid, now string) (DeckRef, error)
	// UpsertSeedCard chèn thẻ nếu chưa có, hồi sinh nếu đang xoá mềm, và
	// hội tụ guid về guid ổn định. Trả true nếu thẻ MỚI (để báo
	// `cards_added`).
	UpsertSeedCard(ctx context.Context, tx Tx, c SeedCard) (bool, error)
	// CountDeckCards đếm thẻ còn sống của deck (báo `cards_total`).
	//
	// Nhận `tx` vì lý do giống `DictHanziSet`: đọc ngoài transaction sẽ trả 0
	// cho lần import đầu tiên vì thẻ vừa chèn chưa commit.
	CountDeckCards(ctx context.Context, tx Tx, deckID int64) (int, error)
}

// DeckRef là deck tối thiểu mà context content cần biết.
type DeckRef struct {
	ID   int64
	Name string
	Lang string
}

// SeedCard là thẻ seed (HSK / PVO / TMRND) — port sang srs, không phải entity
// domain vì nội dung thẻ do context này quyết định còn bản ghi thì srs sở hữu.
type SeedCard struct {
	DeckID int64
	Front  string
	Back   string
	Pinyin string
	Tone   *string
	IPA    *string
	Stress *string
	DueAt  string
	GUID   string
	Now    string
	// TouchUpdated = true cho đường seed: v1 ghi `updated_at` tường minh khi
	// hồi sinh tombstone. Mặc định false để trigger chạm (đồng nhất với
	// luật ghi của M2).
	TouchUpdated bool
}

// ZHEntry là 1 dòng dict (hanzi/pinyin/nghia). Không map `search_vector`:
// cột đó do TRIGGER `trg_dict_search_vector` sinh, GORM không cần biết
// (STACK-V2-PLAN §8: "GORM có thể bypass trigger" — đọc tsvector vào struct
// sẽ khiến GORM thử ghi ngược nó).
type ZHEntry struct {
	Hanzi  string
	Pinyin string
	Nghia  string
}

// ENEntry là 1 dòng en_dict.
type ENEntry struct {
	Lang    string
	Term    string
	Reading string
	Gloss   string
}

// Note là 1 dòng `notes` — bảng dùng chung cho cả 3 context, mỗi context sở
// hữu 1 prefix reserved. `CardID` nil = checklist THIEU (không gắn thẻ).
type Note struct {
	ID        int64
	CardID    *int64
	Text      string
	CreatedAt string
	GUID      string
}

// NowFunc là nguồn thời gian, inject để test được và để 1 request dùng chung
// 1 mốc.
type NowFunc func() time.Time

// Clock mặc định: UTC.
func Clock() time.Time { return time.Now().UTC() }

// NewGUID sinh guid cho row mới.
//
// KHÔNG BAO GIỜ sinh guid rỗng: `ux_notes_guid` / `ux_cards_guid` là UNIQUE
// trên cột `TEXT NOT NULL DEFAULT ”`, nên 2 row cùng guid rỗng là đụng nhau.
// `guid` cũng là natural key để peer union lịch sử — 2 row khác cùng guid rỗng
// sẽ bị coi là 1.
func NewGUID() string { return uuid.NewString() }

// SeedGUIDNamespace là UUID namespace cố định cho guid seed (uuid5/SHA1).
//
// Guid seed PHẢI ỔN ĐỊNH theo natural key: 2 máy cùng import HSK1 cho ra
// cùng guid thì merge khớp mà không nhân đôi (api/sync.go v1 đã dựa vào đúng
// điều này). Guid user (thẻ tạo tay, note) vẫn là uuid4 ngẫu nhiên qua
// NewGUID.
const SeedGUIDNamespace = "9f7c9d2e-6b4a-4e8f-8c1d-3a5b6c7d8e9f"

// SeedDeckGUID là guid ổn định của deck seed theo (name, lang).
func SeedDeckGUID(name, lang string) string {
	return uuid.NewSHA1(uuid.MustParse(SeedGUIDNamespace),
		[]byte("deck:"+name+":"+lang)).String()
}

// SeedCardGUID là guid ổn định của thẻ seed theo (deckName, lang, front).
func SeedCardGUID(deckName, lang, front string) string {
	return uuid.NewSHA1(uuid.MustParse(SeedGUIDNamespace),
		[]byte("card:"+deckName+":"+lang+":"+front)).String()
}
