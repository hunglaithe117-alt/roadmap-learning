package roadmap

import (
	"fmt"
	"strings"
)

// Terrain represents the map terrain theme.
type Terrain string

// TerrainDefault is the fallback terrain.
const TerrainDefault Terrain = "meadow"

const (
	// TerrainMeadow represents meadow terrain.
	TerrainMeadow Terrain = "meadow"
	// TerrainDesert represents desert terrain.
	TerrainDesert Terrain = "desert"
	// TerrainSnow represents snow terrain.
	TerrainSnow Terrain = "snow"
	// TerrainVolcano represents volcano terrain.
	TerrainVolcano Terrain = "volcano"
	// TerrainOcean represents ocean terrain.
	TerrainOcean Terrain = "ocean"
	// TerrainCity represents city terrain.
	TerrainCity Terrain = "city"
)

// AllTerrains lists valid terrain themes.
var AllTerrains = []Terrain{
	TerrainMeadow, TerrainDesert, TerrainSnow,
	TerrainVolcano, TerrainOcean, TerrainCity,
}

// Valid reports whether t is a supported Terrain.
func (t Terrain) Valid() bool {
	for _, v := range AllTerrains {
		if v == t {
			return true
		}
	}
	return false
}

// String returns the terrain string representation.
func (t Terrain) String() string { return string(t) }

var terrainAmplitude = map[Terrain]float64{
	TerrainMeadow:  40,
	TerrainDesert:  25,
	TerrainSnow:    35,
	TerrainVolcano: 70,
	TerrainOcean:   30,
	TerrainCity:    20,
}

// Amplitude returns the relative wave amplitude weight for the terrain.
func (t Terrain) Amplitude() float64 { return terrainAmplitude[t] }

const (
	terrainWaveDepthMin = 0.85
	terrainWaveDepthMax = 1.15
)

// WaveDepth calculates the normalized wave depth factor in [0.85, 1.15].
func (t Terrain) WaveDepth() float64 {
	w := t.Amplitude()
	lo := terrainAmplitude[TerrainCity]
	hi := terrainAmplitude[TerrainVolcano]
	if hi <= lo {
		return 1
	}
	frac := (w - lo) / (hi - lo)
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	return terrainWaveDepthMin + frac*(terrainWaveDepthMax-terrainWaveDepthMin)
}

// Direction represents the map layout flow direction.
type Direction string

// DirectionDefault is the default direction.
const DirectionDefault Direction = "up"

const (
	// DirectionUp flows from bottom to top.
	DirectionUp Direction = "up"
	// DirectionRight flows from left to right.
	DirectionRight Direction = "right"
)

// AllDirections lists valid map flow directions.
var AllDirections = []Direction{DirectionUp, DirectionRight}

// Valid reports whether d is a supported Direction.
func (d Direction) Valid() bool {
	for _, v := range AllDirections {
		if v == d {
			return true
		}
	}
	return false
}

// MapPoint represents 2D coordinates in map viewBox.
type MapPoint struct {
	X float64
	Y float64
}

// ParseTerrain parses raw terrain string into Terrain.
func ParseTerrain(raw string) (Terrain, error) {
	t := Terrain(strings.ToLower(strings.TrimSpace(raw)))
	if t == "" {
		return TerrainDefault, nil
	}
	if !t.Valid() {
		return "", fmt.Errorf("terrain chỉ nhận: %s", strings.Join(terrainNames(), ", "))
	}
	return t, nil
}

// ParseDirection parses raw direction string into Direction.
func ParseDirection(raw string) (Direction, error) {
	d := Direction(strings.ToLower(strings.TrimSpace(raw)))
	if d == "" {
		return DirectionDefault, nil
	}
	if !d.Valid() {
		return "", fmt.Errorf("direction chỉ nhận: %s", strings.Join(directionNames(), ", "))
	}
	return d, nil
}

func terrainNames() []string {
	out := make([]string, 0, len(AllTerrains))
	for _, t := range AllTerrains {
		out = append(out, string(t))
	}
	return out
}

func directionNames() []string {
	out := make([]string, 0, len(AllDirections))
	for _, d := range AllDirections {
		out = append(out, string(d))
	}
	return out
}
