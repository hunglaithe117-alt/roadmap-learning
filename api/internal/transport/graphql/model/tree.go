// Package model provides GraphQL transport view models.
package model

// Path represents a learning path GraphQL model.
type Path struct {
	ID        string `json:"id"`
	GUID      string `json:"guid"`
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	Overview  string `json:"overview"`
	Language  string `json:"language"`
	IsBuiltin bool   `json:"isBuiltin"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// Stage represents a learning path stage GraphQL model.
type Stage struct {
	ID            string    `json:"id"`
	GUID          string    `json:"guid"`
	PathID        string    `json:"pathId"`
	Slug          string    `json:"slug"`
	Title         string    `json:"title"`
	Goal          string    `json:"goal"`
	Position      int       `json:"position"`
	DurationWeeks int       `json:"durationWeeks"`
	Status        Status    `json:"status"`
	StatusNote    string    `json:"statusNote"`
	CompletedAt   *string   `json:"completedAt,omitempty"`
	DeckID        *string   `json:"deckId,omitempty"`
	Terrain       Terrain   `json:"terrain"`
	Direction     Direction `json:"direction"`
	CreatedAt     string    `json:"createdAt"`
	UpdatedAt     string    `json:"updatedAt"`
}

// Topic represents a learning path topic GraphQL model.
type Topic struct {
	ID          string   `json:"id"`
	GUID        string   `json:"guid"`
	StageID     string   `json:"stageId"`
	Title       string   `json:"title"`
	Why         string   `json:"why"`
	Activities  string   `json:"activities"`
	Position    int      `json:"position"`
	Status      Status   `json:"status"`
	StatusNote  string   `json:"statusNote"`
	CompletedAt *string  `json:"completedAt,omitempty"`
	IsOptional  bool     `json:"isOptional"`
	MapX        *float64 `json:"mapX,omitempty"`
	MapY        *float64 `json:"mapY,omitempty"`
	CreatedAt   string   `json:"createdAt"`
	UpdatedAt   string   `json:"updatedAt"`
}
