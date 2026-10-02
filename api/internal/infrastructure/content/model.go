// Package contentinfra là hiện thực GORM của bounded context content: đây là
// nơi DUY NHẤT trong context này được phép import gorm.io
// (STACK-V2-PLAN §2).
package contentinfra

import (
	app "langapp/internal/application/content"
)

// Dict là row bảng `dict` (chữ Hán + pinyin có số thanh + nghĩa).
//
// KHÔNG map cột `search_vector`: cột đó do trigger `trg_dict_search_vector`
// sinh ra, và GORM không cần biết tới nó. Map vào struct nghĩa là GORM sẽ thử
// ghi ngược giá trị trigger vừa tạo (STACK-V2-PLAN §8: "GORM có thể bypass
// trigger"). Cùng lý do `en_dict` không map `search_vector` của nó.
type Dict struct {
	ID     int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Hanzi  string `gorm:"column:hanzi"`
	Pinyin string `gorm:"column:pinyin"`
	Nghia  string `gorm:"column:nghia"`
}

// TableName khoá tên bảng — GORM đoán `content_infra_dicts` từ tên package.
func (Dict) TableName() string { return "dict" }

// EnDict là row bảng `en_dict`.
type EnDict struct {
	ID      int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Lang    string `gorm:"column:lang"`
	Term    string `gorm:"column:term"`
	Reading string `gorm:"column:reading"`
	Gloss   string `gorm:"column:gloss"`
}

// TableName khoá tên bảng (xem Dict.TableName).
func (EnDict) TableName() string { return "en_dict" }

// Note là row bảng `notes` — bảng DÙNG CHUNG cho 3 context, mỗi context 1
// prefix reserved (content: THIEU|, practice: SHADOW|/ERR|). Không có
// `deleted`: note là append-only, xoá là xoá thật.
//
// `CreatedAt string` + `autoCreateTime:false` vì cột là TEXT RFC3339; field
// tên `CreatedAt` kiểu time.Time sẽ bị GORM tự ghi đè bằng giờ máy.
type Note struct {
	ID        int64  `gorm:"column:id;primaryKey;autoIncrement"`
	CardID    *int64 `gorm:"column:card_id"`
	Text      string `gorm:"column:text"`
	CreatedAt string `gorm:"column:created_at;autoCreateTime:false"`
	GUID      string `gorm:"column:guid"`
}

// TableName khoá tên bảng (xem Dict.TableName).
func (Note) TableName() string { return "notes" }

func dictToApp(d Dict) app.ZHEntry {
	return app.ZHEntry{Hanzi: d.Hanzi, Pinyin: d.Pinyin, Nghia: d.Nghia}
}

func enToApp(e EnDict) app.ENEntry {
	return app.ENEntry{Lang: e.Lang, Term: e.Term, Reading: e.Reading, Gloss: e.Gloss}
}

func noteToApp(n Note) app.Note {
	return app.Note{ID: n.ID, CardID: n.CardID, Text: n.Text,
		CreatedAt: n.CreatedAt, GUID: n.GUID}
}
