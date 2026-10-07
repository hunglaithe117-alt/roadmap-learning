package roadmap

// Topic represents a roadmap topic input model for domain policies.
type Topic struct {
	ID          int64
	StageID     int64
	Position    int
	Status      Status
	CompletedAt *string
	IsOptional  IsOptional
	MapX        *float64
	MapY        *float64
	Deleted     int
}
