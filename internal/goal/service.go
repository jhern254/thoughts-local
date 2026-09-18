package goal

import (
	"context"
	"maps"
	"strings"
	"time"
	_ "time/tzdata" // Keep timezone validation independent of host zone files.
	"unicode/utf8"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/validator"
)

type Store interface {
	UpdateGoal(context.Context, *data.Goal) (*data.Goal, error)
	DeleteGoal(context.Context, string, int64, int64) error
	CreateGoal(context.Context, *data.Goal) (*data.Goal, error)
	GetGoal(context.Context, string, int64) (*data.Goal, error)
	ListGoals(context.Context, string) ([]data.Goal, error)
	ListActiveGoals(context.Context, string) ([]data.Goal, error)
	ListInactiveGoals(context.Context, string) ([]data.Goal, error)
}

// CreateGoalInput accepts optional settings; a nil IsActive selects the active default.
type CreateGoalInput struct {
	Name           string
	TargetSeconds  int64
	Priority       string
	StartDate      string
	EndDate        string
	IsActive       *bool
	Cadence        string
	TZ             string
	WeekStart      string
	DefaultCadence string
}

// UpdateGoalInput replaces editable settings; nil IsActive and Priority preserve their values.
// Empty optional dates and default cadence clear those settings.
// Cadence, TZ, and WeekStart must be explicit; creation defaults do not apply.
type UpdateGoalInput struct {
	Name           string
	TargetSeconds  int64
	Priority       *string
	StartDate      string
	EndDate        string
	IsActive       *bool
	Cadence        string
	TZ             string
	WeekStart      string
	DefaultCadence string
}

type ValidationError struct {
	Fields       map[string]string
	publicFields map[string]string
}

func (e *ValidationError) Error() string { return "goal validation failed" }

// PublicFields returns fixed validator guidance independent of mutable Fields.
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

func NewService(store Store) *Service { return &Service{store: store, now: time.Now} }

func (s *Service) Get(ctx context.Context, userID string, id int64) (*data.Goal, error) {
	return s.store.GetGoal(ctx, userID, id)
}

func (s *Service) List(ctx context.Context, userID string) ([]data.Goal, error) {
	return s.store.ListGoals(ctx, userID)
}

func (s *Service) ListActive(ctx context.Context, userID string) ([]data.Goal, error) {
	return s.store.ListActiveGoals(ctx, userID)
}

func (s *Service) ListInactive(ctx context.Context, userID string) ([]data.Goal, error) {
	return s.store.ListInactiveGoals(ctx, userID)
}

func (s *Service) Create(ctx context.Context, userID string, input CreateGoalInput) (*data.Goal, error) {
	now := s.now().UTC().Truncate(time.Second)
	item := &data.Goal{
		UserID: userID, GoalName: strings.Trim(input.Name, " "), TargetSeconds: input.TargetSeconds,
		StartDate: optionalSetting(input.StartDate), EndDate: optionalSetting(input.EndDate),
		IsActive: true, Cadence: strings.Trim(input.Cadence, " "), TZ: strings.Trim(input.TZ, " "),
		WeekStart: strings.Trim(input.WeekStart, " "), DefaultCadence: optionalSetting(input.DefaultCadence),
		Priority: strings.Trim(input.Priority, " "), Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if input.IsActive != nil {
		item.IsActive = *input.IsActive
	}
	if item.Priority == "" {
		item.Priority = "normal"
	}
	if item.Cadence == "" {
		item.Cadence = "weekly"
	}
	if item.TZ == "" {
		item.TZ = "UTC"
	}
	if item.WeekStart == "" {
		item.WeekStart = "mon"
	}
	if err := validateGoal(item, &item.Priority); err != nil {
		return nil, err
	}
	return s.store.CreateGoal(ctx, item)
}

// Update replaces settings using the caller's version for optimistic concurrency.
func (s *Service) Update(ctx context.Context, userID string, goalID, expectedVersion int64, input UpdateGoalInput) (*data.Goal, error) {
	item := &data.Goal{
		GoalID: goalID, UserID: userID, GoalName: strings.Trim(input.Name, " "), TargetSeconds: input.TargetSeconds,
		StartDate: optionalSetting(input.StartDate), EndDate: optionalSetting(input.EndDate),
		Cadence: strings.Trim(input.Cadence, " "), TZ: strings.Trim(input.TZ, " "),
		WeekStart: strings.Trim(input.WeekStart, " "), DefaultCadence: optionalSetting(input.DefaultCadence),
		Version: expectedVersion, UpdatedAt: s.now().UTC().Truncate(time.Second),
	}
	var priority *string
	if input.Priority != nil {
		item.Priority = strings.Trim(*input.Priority, " ")
		priority = &item.Priority
	}
	if err := validateGoal(item, priority); err != nil {
		return nil, err
	}
	if input.IsActive != nil {
		item.IsActive = *input.IsActive
	}
	if input.IsActive == nil || input.Priority == nil {
		current, err := s.store.GetGoal(ctx, userID, goalID)
		if err != nil {
			return nil, err
		}
		if input.IsActive == nil {
			item.IsActive = current.IsActive
		}
		if input.Priority == nil {
			item.Priority = current.Priority
		}
	}
	// Keep the caller's version so a concurrent change still rejects the write.
	return s.store.UpdateGoal(ctx, item)
}

// Delete hides the goal while retaining its record, activation state, and history.
func (s *Service) Delete(ctx context.Context, userID string, goalID, expectedVersion int64) error {
	v := validator.NewValidator()
	v.Check(userID != "", "user_id", "must be provided")
	v.Check(expectedVersion > 0, "version", "must be greater than zero")
	if !v.Valid() {
		return &ValidationError{Fields: v.Errors, publicFields: maps.Clone(v.Errors)}
	}
	return s.store.DeleteGoal(ctx, userID, goalID, expectedVersion)
}

func optionalSetting(value string) *string {
	value = strings.Trim(value, " ")
	if value == "" {
		return nil
	}
	return &value
}

// A nil priority skips validation of an omitted update field, which is loaded later.
func validateGoal(item *data.Goal, priority *string) error {
	v := validator.NewValidator()
	if priority != nil {
		v.Check(validator.In(*priority, "low", "normal", "high"), "priority", "must be low, normal, or high")
	}
	v.Check(item.UserID != "", "user_id", "must be provided")
	v.Check(item.Version > 0, "version", "must be greater than zero")
	// SQLite length(TEXT) counts code points up to the first NUL.
	name, _, _ := strings.Cut(item.GoalName, "\x00")
	length := utf8.RuneCountInString(name)
	v.Check(length >= 1 && length <= 512, "goal_name", "must be between 1 and 512 characters long")
	v.Check(item.TargetSeconds > 0, "target_seconds", "must be greater than zero")
	v.Check(validCadence(item.Cadence), "cadence", "must be daily, weekly, monthly, mtd, quarterly, or yearly")
	if item.DefaultCadence != nil {
		v.Check(validCadence(*item.DefaultCadence), "default_cadence", "must be daily, weekly, monthly, mtd, quarterly, or yearly")
	}
	v.Check(validator.In(item.WeekStart, "mon", "tue", "wed", "thu", "fri", "sat", "sun"), "week_start", "must be mon, tue, wed, thu, fri, sat, or sun")
	v.Check(utf8.RuneCountInString(item.TZ) <= 64, "tz", "must not be more than 64 characters long")
	_, zoneErr := time.LoadLocation(item.TZ)
	v.Check(item.TZ != "" && item.TZ != "Local" && zoneErr == nil, "tz", "must be UTC or a recognized named timezone")
	startValid, endValid := validDate(item.StartDate), validDate(item.EndDate)
	v.Check(startValid, "goal_start_date", "must be a valid calendar date in YYYY-MM-DD format")
	v.Check(endValid, "goal_end_date", "must be a valid calendar date in YYYY-MM-DD format")
	if startValid && endValid && item.StartDate != nil && item.EndDate != nil {
		v.Check(*item.EndDate >= *item.StartDate, "goal_end_date", "must not be before the start date")
	}
	if !v.Valid() {
		return &ValidationError{Fields: v.Errors, publicFields: maps.Clone(v.Errors)}
	}
	return nil
}

func validCadence(value string) bool {
	return validator.In(value, "daily", "weekly", "monthly", "mtd", "quarterly", "yearly")
}

func validDate(value *string) bool {
	if value == nil {
		return true
	}
	parsed, err := time.Parse(time.DateOnly, *value)
	return err == nil && parsed.Format(time.DateOnly) == *value
}
