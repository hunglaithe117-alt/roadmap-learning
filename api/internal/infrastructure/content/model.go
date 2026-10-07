// Package contentinfra provides GORM models and repository implementations for content.
package contentinfra

import (
	app "langapp/internal/application/content"
)

// Dict represents a database row in the dict table.
type Dict struct {
	ID     int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Hanzi  string `gorm:"column:hanzi"`
	Pinyin string `gorm:"column:pinyin"`
	Nghia  string `gorm:"column:nghia"`
}

// TableName returns the table name for Dict.
func (Dict) TableName() string { return "dict" }

// EnDict represents a database row in the en_dict table.
type EnDict struct {
	ID      int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Lang    string `gorm:"column:lang"`
	Term    string `gorm:"column:term"`
	Reading string `gorm:"column:reading"`
	Gloss   string `gorm:"column:gloss"`
}

// TableName returns the table name for EnDict.
func (EnDict) TableName() string { return "en_dict" }

// Note represents a database row in the notes table.
type Note struct {
	ID        int64  `gorm:"column:id;primaryKey;autoIncrement"`
	CardID    *int64 `gorm:"column:card_id"`
	Text      string `gorm:"column:text"`
	CreatedAt string `gorm:"column:created_at;autoCreateTime:false"`
	GUID      string `gorm:"column:guid"`
}

// TableName returns the table name for Note.
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
