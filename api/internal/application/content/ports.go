// Package content provides application orchestration for dictionaries,
// HSK seed importing, tone processing, chunking, stress analysis, and THIEU checklists.
package content

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Tx represents a database transaction handle.
type Tx = any

// UnitOfWork runs an operation inside a database transaction.
type UnitOfWork interface {
	Do(ctx context.Context, fn func(tx Tx) error) error
}

// Repository defines data access operations for content.
type Repository interface {
	SearchZH(ctx context.Context, q string, limit int) ([]ZHEntry, error)
	SearchEN(ctx context.Context, q string, limit int) ([]ENEntry, error)
	LookupEN(ctx context.Context, term string) (ENEntry, bool, error)
	DictHanziSet(ctx context.Context, tx Tx) (map[string]bool, error)
	InsertDict(ctx context.Context, tx Tx, e *ZHEntry) (bool, error)
	CountDict(ctx context.Context) (int, error)

	InsertEN(ctx context.Context, tx Tx, e *ENEntry) (bool, error)
	CountEN(ctx context.Context) (int, error)

	InsertNote(ctx context.Context, tx Tx, n *Note) error
	ListNotesByPrefix(ctx context.Context, prefix string, limit int) ([]Note, error)
}

// CardToneWriter updates tone results on cards.
type CardToneWriter interface {
	SetCardTone(ctx context.Context, tx Tx, cardID int64, tone *string) (string, error)
}

// CardReader checks card existence.
type CardReader interface {
	CardExists(ctx context.Context, id int64) (bool, error)
}

// DeckWriter creates decks and cards for content seeding.
type DeckWriter interface {
	EnsureSeedDeck(ctx context.Context, tx Tx, name, lang, guid, now string) (DeckRef, error)
	UpsertSeedCard(ctx context.Context, tx Tx, c SeedCard) (bool, error)
	CountDeckCards(ctx context.Context, tx Tx, deckID int64) (int, error)
}

// DeckRef represents minimal deck metadata.
type DeckRef struct {
	ID   int64
	Name string
	Lang string
}

// SeedCard holds seed card fields for importing.
type SeedCard struct {
	DeckID       int64
	Front        string
	Back         string
	Pinyin       string
	Tone         *string
	IPA          *string
	Stress       *string
	DueAt        string
	GUID         string
	Now          string
	TouchUpdated bool
}

// ZHEntry represents a Chinese dictionary entry.
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

// NowFunc provides current time, injected for deterministic tests.
type NowFunc func() time.Time

// Clock returns the current UTC time.
func Clock() time.Time { return time.Now().UTC() }

// NewGUID generates a new UUID v4 string.
func NewGUID() string { return uuid.NewString() }

// SeedGUIDNamespace is the fixed UUID namespace for deterministic seed GUIDs.
const SeedGUIDNamespace = "9f7c9d2e-6b4a-4e8f-8c1d-3a5b6c7d8e9f"

// SeedDeckGUID generates a stable seed deck GUID based on (name, lang).
func SeedDeckGUID(name, lang string) string {
	return uuid.NewSHA1(uuid.MustParse(SeedGUIDNamespace),
		[]byte("deck:"+name+":"+lang)).String()
}

// SeedCardGUID generates a stable seed card GUID based on (deckName, lang, front).
func SeedCardGUID(deckName, lang, front string) string {
	return uuid.NewSHA1(uuid.MustParse(SeedGUIDNamespace),
		[]byte("card:"+deckName+":"+lang+":"+front)).String()
}
