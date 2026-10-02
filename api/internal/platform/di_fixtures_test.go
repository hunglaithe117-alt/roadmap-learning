package platform_test

import (
	"io"
	"log/slog"

	contentapp "langapp/internal/application/content"
	roadmapapp "langapp/internal/application/roadmap"
)

// Input dùng chung cho test DI — gom ở đây để test đọc được phần "kịch bản"
// mà không phải lướt qua 6 struct input của application.

func pathInput(slug string) roadmapapp.PathInput {
	return roadmapapp.PathInput{Slug: slug, Title: "Chinese " + slug, Language: "zh"}
}

func stageInputWithDeck(deckID int64) roadmapapp.StageInput {
	return roadmapapp.StageInput{Slug: "g1", Title: "Giai đoạn 1", DeckID: &deckID}
}

func hskImport() contentapp.ImportInput {
	return contentapp.ImportInput{Level: "HSK1"}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}
