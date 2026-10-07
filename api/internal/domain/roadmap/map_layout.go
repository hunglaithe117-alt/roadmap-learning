package roadmap

import (
	"math"
	"sort"
)

// Map layout geometric constants.
const (
	// MapCanvasHeight is the fixed cross-axis canvas height.
	MapCanvasHeight = 900.0

	// MapMargin is the margin at both ends of the main axis.
	MapMargin = 80.0

	// MapNodeSpacing is the distance between consecutive nodes along the main axis.
	MapNodeSpacing = 440.0

	// MapWaveAmplitudeRatio is the wave amplitude ratio relative to MapCanvasHeight.
	MapWaveAmplitudeRatio = 0.30

	// MapMinWiggleRatio is the minimum (maxY-minY)/MapCanvasHeight wiggle ratio.
	MapMinWiggleRatio = 0.35

	// MapMinLandscapeAspect is the required width/height aspect ratio when n >= 4.
	MapMinLandscapeAspect = 1.6

	// MapMaxNodes is the maximum node ceiling used for manual coordinate bounds checks.
	MapMaxNodes = 60

	// MapViewWidth and MapViewHeight define bounds for manual coordinate validation.
	MapViewWidth  = 2*MapMargin + (MapMaxNodes-1)*MapNodeSpacing
	MapViewHeight = MapCanvasHeight
)

// MapCanvasWidth calculates the canvas width for a given node count.
func MapCanvasWidth(nodeCount int) float64 {
	if nodeCount <= 1 {
		return 2 * MapMargin
	}
	return 2*MapMargin + float64(nodeCount-1)*MapNodeSpacing
}

// SortedTopics returns a copy of topics stably sorted by (Position, ID).
func SortedTopics(topics []Topic) []Topic {
	out := make([]Topic, len(topics))
	copy(out, topics)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Position != out[j].Position {
			return out[i].Position < out[j].Position
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// LevelState represents the visual display state of a node on the map.
type LevelState string

const (
	// LevelDone indicates completed or skipped levels.
	LevelDone LevelState = "done"
	// LevelCurrent indicates the active unlocked level.
	LevelCurrent LevelState = "current"
	// LevelLocked indicates a locked level.
	LevelLocked LevelState = "locked"
)

// AllLevelStates lists valid level states.
var AllLevelStates = []LevelState{LevelDone, LevelCurrent, LevelLocked}

// LevelStates derives display states for topics in ascending position order.
func LevelStates(topics []Topic) []LevelState {
	ordered := SortedTopics(topics)
	out := make([]LevelState, len(ordered))
	frontierOpen := true
	for i, t := range ordered {
		switch {
		case t.Status.IsTerminal():
			out[i] = LevelDone
			frontierOpen = true
		case frontierOpen:
			out[i] = LevelCurrent
			frontierOpen = false
		default:
			out[i] = LevelLocked
		}
	}
	return out
}

// ComputeLayout calculates map coordinates for topics using sine wave curves.
// User-defined MapX/MapY override procedural coordinates.
func ComputeLayout(topics []Topic, terrain Terrain, dir Direction) []MapPoint {
	ordered := SortedTopics(topics)
	if len(ordered) == 0 {
		return []MapPoint{}
	}
	if !terrain.Valid() {
		terrain = TerrainDefault
	}
	if !dir.Valid() {
		dir = DirectionDefault
	}

	n := len(ordered)
	alongSpan := MapCanvasWidth(n)
	acrossCenter := MapCanvasHeight / 2
	amp := MapWaveAmplitudeRatio * MapCanvasHeight * terrain.WaveDepth()
	periods := float64(WavePeriods(n))

	points := make([]MapPoint, n)
	for i, t := range ordered {
		wave := math.Sin(2 * math.Pi * periods * (float64(i) + 0.5) / float64(n))
		along := MapMargin + float64(i)*MapNodeSpacing
		across := acrossCenter + amp*wave

		var auto MapPoint
		if dir == DirectionUp {
			auto = MapPoint{X: across, Y: alongSpan - along}
		} else {
			auto = MapPoint{X: along, Y: across}
		}

		points[i] = overrideWithUserPoint(auto, t)
	}
	return points
}

// WavePeriods returns the number of sine wave periods for n nodes: max(1, round((n-1)/4)).
func WavePeriods(n int) int {
	if n < 2 {
		return 1
	}
	p := int(math.Round(float64(n-1) / 4))
	if p < 1 {
		p = 1
	}
	return p
}

func overrideWithUserPoint(auto MapPoint, t Topic) MapPoint {
	if t.MapX != nil {
		auto.X = *t.MapX
	}
	if t.MapY != nil {
		auto.Y = *t.MapY
	}
	return auto
}
