package graphql

import (
	contentapp "langapp/internal/application/content"
	insightapp "langapp/internal/application/insight"
	practiceapp "langapp/internal/application/practice"
	roadmapapp "langapp/internal/application/roadmap"
	srsapp "langapp/internal/application/srs"
	syncapp "langapp/internal/application/sync"
)

// Resolver is the root resolver struct holding application service dependencies.
type Resolver struct {
	SRS      *srsapp.Service
	Content  *contentapp.Service
	Roadmap  *roadmapapp.Service
	Practice *practiceapp.Service
	Insight  *insightapp.Service
	Sync     *syncapp.Service
}

// NewResolver constructs the root resolver with application services.
func NewResolver(
	srs *srsapp.Service,
	content *contentapp.Service,
	roadmap *roadmapapp.Service,
	practice *practiceapp.Service,
	insight *insightapp.Service,
	sync *syncapp.Service,
) *Resolver {
	return &Resolver{SRS: srs, Content: content, Roadmap: roadmap, Practice: practice, Insight: insight, Sync: sync}
}
