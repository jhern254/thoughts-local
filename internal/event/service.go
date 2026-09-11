package event

import (
	"context"
	"maps"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/validator"
)

type Store interface {
	GetEvent(context.Context, string, int64) (*data.Event, error)
	StartEvent(context.Context, *data.Event) (*data.Event, error)
	AddPastEvent(context.Context, *data.Event) (*data.Event, error)
	ListEvents(context.Context, string, time.Time, time.Time) ([]data.Event, error)
	EndEvent(context.Context, string, int64, int64, time.Time, time.Time) (*data.Event, error)
	UpdateEvent(context.Context, *data.Event) (*data.Event, error)
}

type ValidationError struct {
	Fields       map[string]string
	publicFields map[string]string
}

func (e *ValidationError) Error() string { return "event validation failed" }

// PublicFields returns only fixed validator guidance, independent of mutable Fields.
func (e *ValidationError) PublicFields() map[string]string {
	if e == nil {
		return nil
	}
	return maps.Clone(e.publicFields)
}

type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store) *Service {
	return &Service{store: store, now: time.Now}
}

func (s *Service) Get(ctx context.Context, userID string, eventID int64) (*data.Event, error) {
	return s.store.GetEvent(ctx, userID, eventID)
}

// End finishes an ongoing event. A zero end requests the current time.
// The store checks the persisted start, lifecycle and version atomically.
func (s *Service) End(ctx context.Context, userID string, eventID, version int64, endedAt time.Time) (*data.Event, error) {
	now := s.now().UTC().Truncate(time.Second)
	if endedAt.IsZero() {
		endedAt = now
	}
	endedAt = endedAt.UTC().Truncate(time.Second)
	v := validator.NewValidator()
	v.Check(userID != "", "user_id", "must be provided")
	v.Check(version > 0, "version", "must be greater than zero")
	v.Check(!endedAt.After(now), "ended_at", "must not be in the future")
	if !v.Valid() {
		return nil, &ValidationError{Fields: v.Errors, publicFields: maps.Clone(v.Errors)}
	}
	return s.store.EndEvent(ctx, userID, eventID, version, endedAt, now)
}

// Update corrects label and explicit timestamps without changing lifecycle state.
// A nil end keeps an ongoing event ongoing; completed events require an end.
func (s *Service) Update(ctx context.Context, userID string, eventID, version int64, activity string, startedAt time.Time, endedAt *time.Time) (*data.Event, error) {
	now := s.now().UTC().Truncate(time.Second)
	item := newEvent(userID, activity, startedAt, now)
	item.EventID, item.Version = eventID, version
	if endedAt != nil {
		end := endedAt.UTC().Truncate(time.Second)
		item.EndedAt = &end
	}
	if err := validateEvent(item, now); err != nil {
		return nil, err
	}
	return s.store.UpdateEvent(ctx, item)
}

// Create starts an ongoing event, atomically closing its predecessor in the store.
// A zero start requests the current time; supplied starts may be backdated.
func (s *Service) Create(ctx context.Context, userID, activity string, startedAt time.Time) (*data.Event, error) {
	now := s.now().UTC().Truncate(time.Second)
	if startedAt.IsZero() {
		startedAt = now
	}
	item := newEvent(userID, activity, startedAt, now)
	if err := validateEvent(item, now); err != nil {
		return nil, err
	}
	return s.store.StartEvent(ctx, item)
}

// CreatePast requires explicit timestamps and never changes the ongoing event.
func (s *Service) CreatePast(ctx context.Context, userID, activity string, startedAt, endedAt time.Time) (*data.Event, error) {
	now := s.now().UTC().Truncate(time.Second)
	item := newEvent(userID, activity, startedAt, now)
	end := endedAt.UTC().Truncate(time.Second)
	item.EndedAt = &end
	if err := validateEvent(item, now); err != nil {
		return nil, err
	}
	return s.store.AddPastEvent(ctx, item)
}

// List accepts explicit instants, not calendar dates. Future bounds are valid.
func (s *Service) List(ctx context.Context, userID string, from, until time.Time) ([]data.Event, error) {
	from, until = from.UTC().Truncate(time.Second), until.UTC().Truncate(time.Second)
	v := validator.NewValidator()
	v.Check(!from.IsZero(), "from", "must be provided")
	v.Check(!until.IsZero(), "until", "must be provided")
	v.Check(until.After(from), "until", "must be after from")
	if !v.Valid() {
		return nil, &ValidationError{Fields: v.Errors, publicFields: maps.Clone(v.Errors)}
	}
	return s.store.ListEvents(ctx, userID, from, until)
}

func newEvent(userID, activity string, start, now time.Time) *data.Event {
	item := &data.Event{UserID: userID, StartedAt: start.UTC().Truncate(time.Second),
		CreatedAt: now, UpdatedAt: now, Version: 1}
	if activity = strings.Trim(activity, " "); activity != "" {
		item.ActivityType = &activity
	}
	return item
}

func validateEvent(item *data.Event, now time.Time) error {
	v := validator.NewValidator()
	v.Check(item.UserID != "", "user_id", "must be provided")
	v.Check(item.Version > 0, "version", "must be greater than zero")
	v.Check(!item.StartedAt.IsZero(), "started_at", "must be provided")
	v.Check(!item.StartedAt.After(now), "started_at", "must not be in the future")
	if item.EndedAt != nil {
		v.Check(!item.EndedAt.IsZero(), "ended_at", "must be provided")
		v.Check(!item.EndedAt.After(now), "ended_at", "must not be in the future")
		v.Check(!item.EndedAt.Before(item.StartedAt), "ended_at", "must not be before started_at")
	}
	if item.ActivityType != nil {
		// SQLite length(TEXT) counts code points only up to the first NUL.
		// Match its CHECK without shortening the stored label.
		prefix, _, _ := strings.Cut(*item.ActivityType, "\x00")
		length := utf8.RuneCountInString(prefix)
		v.Check(length >= 1 && length <= 4096, "activity_type", "must contain between 1 and 4096 characters")
	}
	if !v.Valid() {
		return &ValidationError{Fields: v.Errors, publicFields: maps.Clone(v.Errors)}
	}
	return nil
}
