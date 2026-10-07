package roadmap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	domain "langapp/internal/domain/roadmap"
	"langapp/internal/typednil"
)

// Service provides use case orchestration for learning paths and progress.
type Service struct {
	repo    Repository
	uow     UnitOfWork
	decks   DeckReader
	nowFn   NowFunc
	viewBox ViewBox
}

// ViewBox represents the roadmap map canvas boundaries.
type ViewBox struct {
	Width  float64
	Height float64
}

// NewService constructs a roadmap Service.
func NewService(repo Repository, uow UnitOfWork, decks DeckReader, nowFn NowFunc, viewBox ViewBox) *Service {
	if nowFn == nil {
		nowFn = Clock
	}
	if viewBox.Width == 0 || viewBox.Height == 0 {
		viewBox = ViewBox{Width: domain.MapViewWidth, Height: domain.MapViewHeight}
	}
	return &Service{repo: repo, uow: uow, decks: decks, nowFn: nowFn, viewBox: viewBox}
}

// timestamp formats the current UTC time as an RFC3339 string.
func (s *Service) timestamp() string { return s.nowFn().UTC().Format(time.RFC3339) }

// parseMapField converts terrain or direction parsing errors into bad request errors.
func parseMapField[T ~string](raw string, parse func(string) (T, error)) (string, error) {
	v, err := parse(raw)
	if err != nil {
		return "", newError(StatusBadRequest, "%s", err.Error())
	}
	return string(v), nil
}

// parseOptionalStatus parses and validates optional status strings, defaulting to not_started.
func parseOptionalStatus(in *string) (Status, error) {
	if in == nil || strings.TrimSpace(*in) == "" {
		return StatusNotStarted, nil
	}
	s := strings.TrimSpace(*in)
	if !ValidStatus(s) {
		return "", newError(StatusBadRequest, "status chỉ nhận: not_started, in_progress, done, skipped")
	}
	return s, nil
}

// resolveDeck validates deck_id via DeckReader and checks language compatibility.
func (s *Service) resolveDeck(ctx context.Context, pathLang string, id *int64) (*DeckInfo, error) {
	if id == nil || *id <= 0 {
		return nil, nil
	}
	if typednil.Is(s.decks) {
		return nil, newError(StatusInternalServerError, "chưa sẵn sàng kiểm tra deck")
	}
	info, err := s.decks.Find(ctx, *id)
	if err != nil {
		return nil, fmt.Errorf("kiểm tra deck: %w", err)
	}
	if !info.Exists {
		return nil, newError(StatusBadRequest, "không tìm thấy deck")
	}
	if pathFrozen, ok := frozenLang(pathLang); ok {
		if deckLang, ok := frozenLang(info.Lang); ok && deckLang != pathFrozen {
			return nil, newError(StatusBadRequest,
				"deck ngôn ngữ %s không khớp learning path ngôn ngữ %s", deckLang, pathFrozen)
		}
	}
	return &DeckInfo{Exists: true, Name: info.Name, Lang: info.Lang}, nil
}

// frozenLang maps a language code to the frozen set {"zh", "en"}.
func frozenLang(l string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(l)) {
	case "zh":
		return "zh", true
	case "en":
		return "en", true
	default:
		return "", false
	}
}

// nextPosition determines the next position integer.
func nextPosition(given *int, last int) (int, error) {
	if given != nil {
		if err := ValidatePosition(*given); err != nil {
			return 0, err
		}
		return *given, nil
	}
	return last + 1, nil
}

func (s *Service) nextStagePosition(ctx context.Context, pathID int64, given *int) (int, error) {
	last, err := s.repo.MaxStagePosition(ctx, pathID)
	if err != nil {
		return 0, fmt.Errorf("tính position của stage: %w", err)
	}
	return nextPosition(given, last)
}

func (s *Service) nextTopicPosition(ctx context.Context, stageID int64, given *int) (int, error) {
	last, err := s.repo.MaxTopicPosition(ctx, stageID)
	if err != nil {
		return 0, fmt.Errorf("tính position của topic: %w", err)
	}
	return nextPosition(given, last)
}

func (s *Service) nextResourcePosition(ctx context.Context, topicID int64, given *int) (int, error) {
	last, err := s.repo.MaxResourcePosition(ctx, topicID)
	if err != nil {
		return 0, fmt.Errorf("tính position của resource: %w", err)
	}
	return nextPosition(given, last)
}

func (s *Service) nextMilestonePosition(ctx context.Context, stageID int64, given *int) (int, error) {
	last, err := s.repo.MaxMilestonePosition(ctx, stageID)
	if err != nil {
		return 0, fmt.Errorf("tính position của milestone: %w", err)
	}
	return nextPosition(given, last)
}

func (s *Service) validateMapCoord(v *float64, name string, max float64) (*float64, error) {
	return ValidateMapCoord(v, name, max)
}

func (s *Service) inTx(ctx context.Context, fn func(tx Tx) error) error {
	if typednil.Is(s.uow) {
		return fn(nil)
	}
	return s.uow.Do(ctx, fn)
}

// wrapNotFound converts ErrNotFound into a 404 Error.
func wrapNotFound(err error, msg string) error {
	if errors.Is(err, ErrNotFound) {
		return newError(StatusNotFound, "%s", msg)
	}
	return err
}

// isUniqueViolation checks whether an error is a database unique constraint violation.
func isUniqueViolation(err error) bool {
	return err != nil &&
		(strings.Contains(err.Error(), "23505") || strings.Contains(err.Error(), "UNIQUE constraint"))
}

// EncodeActivities encodes activities into a JSON array string.
func EncodeActivities(in []string) string {
	out := make([]string, 0, len(in))
	for _, a := range in {
		if a = strings.TrimSpace(a); a != "" {
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		return ""
	}
	b, err := json.Marshal(out)
	if err != nil {
		return strings.Join(out, "\n")
	}
	return string(b)
}
