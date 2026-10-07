package graphql

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/vikstrous/dataloadgen"

	roadmapapp "langapp/internal/application/roadmap"
	srsapp "langapp/internal/application/srs"
	domainroadmap "langapp/internal/domain/roadmap"
)

// Loaders holds per-request dataloaders for the roadmap hierarchy.
const loaderWait = time.Millisecond

// Loaders batches database reads for the roadmap tree across resolvers.
type Loaders struct {
	stageTree *dataloadgen.Loader[int64, []roadmapapp.StageView]
	topicRes  *dataloadgen.Loader[int64, []roadmapapp.Resource]
	deckRef   *dataloadgen.Loader[int64, *roadmapapp.DeckRef]
	progress  *dataloadgen.Loader[int64, roadmapapp.Progress]

	topicMeta   map[int64]topicMeta
	topicMetaMu sync.RWMutex
}

// topicMeta stores calculated layout metadata for a topic.
type topicMeta struct {
	Level     domainroadmap.LevelState
	Point     domainroadmap.MapPoint
	MapPinned bool
}

type loadersKey struct{}

// NewLoaders initializes dataloaders for a request using application services.
func NewLoaders(roadmap *roadmapapp.Service, srs *srsapp.Service) *Loaders {
	return &Loaders{
		topicMeta: map[int64]topicMeta{},
		stageTree: dataloadgen.NewMappedLoader(
			func(ctx context.Context, pathIDs []int64) (map[int64][]roadmapapp.StageView, error) {
				return roadmap.StageTreeByPathIDs(ctx, pathIDs)
			},
			dataloadgen.WithWait(loaderWait),
		),
		topicRes: dataloadgen.NewMappedLoader(
			func(ctx context.Context, topicIDs []int64) (map[int64][]roadmapapp.Resource, error) {
				return roadmap.ResourcesByTopicIDs(ctx, topicIDs)
			},
			dataloadgen.WithWait(loaderWait),
		),
		deckRef: dataloadgen.NewMappedLoader(
			func(ctx context.Context, deckIDs []int64) (map[int64]*roadmapapp.DeckRef, error) {
				infos, err := srs.FindDecks(ctx, deckIDs)
				if err != nil {
					return nil, err
				}
				out := make(map[int64]*roadmapapp.DeckRef, len(infos))
				for id, info := range infos {
					if !info.Exists {
						continue
					}
					out[id] = &roadmapapp.DeckRef{ID: id, Name: info.Name, Lang: info.Lang}
				}
				return out, nil
			},
			dataloadgen.WithWait(loaderWait),
		),
		progress: dataloadgen.NewMappedLoader(
			func(ctx context.Context, pathIDs []int64) (map[int64]roadmapapp.Progress, error) {
				return roadmap.ProgressByPathIDs(ctx, pathIDs)
			},
			dataloadgen.WithWait(loaderWait),
		),
	}
}

// LoadStages batches loading of stage views for a roadmap path.
func (l *Loaders) LoadStages(ctx context.Context, pathID int64) ([]roadmapapp.StageView, error) {
	out, err := l.stageTree.Load(ctx, pathID)
	if errors.Is(err, dataloadgen.ErrNotFound) {
		return []roadmapapp.StageView{}, nil
	}
	return out, err
}

// LoadResources batches loading of resources for a topic.
func (l *Loaders) LoadResources(ctx context.Context, topicID int64) ([]roadmapapp.Resource, error) {
	out, err := l.topicRes.Load(ctx, topicID)
	if errors.Is(err, dataloadgen.ErrNotFound) {
		return []roadmapapp.Resource{}, nil
	}
	return out, err
}

// LoadDeck batches loading of deck references for a stage.
func (l *Loaders) LoadDeck(ctx context.Context, deckID int64) (*roadmapapp.DeckRef, error) {
	out, err := l.deckRef.Load(ctx, deckID)
	if errors.Is(err, dataloadgen.ErrNotFound) {
		return nil, nil
	}
	return out, err
}

// LoadProgress batches loading of progress for a roadmap path.
func (l *Loaders) LoadProgress(ctx context.Context, pathID int64) (roadmapapp.Progress, error) {
	out, err := l.progress.Load(ctx, pathID)
	if errors.Is(err, dataloadgen.ErrNotFound) {
		return roadmapapp.Progress{}, nil
	}
	return out, err
}

// rememberTopic caches layout metadata for topic views.
func (l *Loaders) rememberTopic(views []roadmapapp.TopicView) {
	l.topicMetaMu.Lock()
	defer l.topicMetaMu.Unlock()
	for _, tv := range views {
		l.topicMeta[tv.ID] = topicMeta{
			Level: tv.Level, Point: tv.Point, MapPinned: tv.MapPinned,
		}
	}
}

// TopicMeta returns the cached layout metadata for a topic.
func (l *Loaders) TopicMeta(topicID int64) topicMeta {
	l.topicMetaMu.RLock()
	defer l.topicMetaMu.RUnlock()
	if m, ok := l.topicMeta[topicID]; ok {
		return m
	}
	return topicMeta{
		Level: domainroadmap.LevelCurrent,
		Point: domainroadmap.MapPoint{},
	}
}

// Middleware injects a fresh Loaders instance into the request context.
func Middleware(roadmap *roadmapapp.Service, srs *srsapp.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loaders := NewLoaders(roadmap, srs)
		ctx := context.WithValue(r.Context(), loadersKey{}, loaders)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// LoadersFrom extracts the Loaders instance from the context or initializes a fallback.
func LoadersFrom(ctx context.Context, roadmap *roadmapapp.Service, srs *srsapp.Service) *Loaders {
	if l, ok := ctx.Value(loadersKey{}).(*Loaders); ok {
		return l
	}
	return NewLoaders(roadmap, srs)
}
