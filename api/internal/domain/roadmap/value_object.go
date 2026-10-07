// Package roadmap provides domain models, status transition logic, and layout calculations for learning roadmaps.
package roadmap

// Status represents the completion status of a roadmap node.
type Status string

const (
	// NotStarted indicates the topic has not been started.
	NotStarted Status = "not_started"
	// InProgress indicates the topic is currently being studied.
	InProgress Status = "in_progress"
	// Done indicates the topic is completed.
	Done Status = "done"
	// Skipped indicates the topic was skipped.
	Skipped Status = "skipped"
)

// AllStatuses lists valid topic statuses.
var AllStatuses = []Status{NotStarted, InProgress, Done, Skipped}

// Valid reports whether s is a valid Status.
func (s Status) Valid() bool {
	for _, v := range AllStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// IsTerminal reports whether s is terminal (Done or Skipped).
func (s Status) IsTerminal() bool { return s == Done || s == Skipped }

// IsOptional flags whether a node is optional and excluded from progress calculations.
type IsOptional bool

const (
	// Required marks an obligatory topic.
	Required IsOptional = false
	// Optional marks an elective topic.
	Optional IsOptional = true
)
