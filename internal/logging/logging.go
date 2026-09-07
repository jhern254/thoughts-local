package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

const callerSkipFrameCount = 3

type MutationEvent uint8

const (
	SubjectCreated MutationEvent = iota
	SubjectUpdated
	SubjectDeleted
)

type Operation uint8

const (
	ApplicationStart Operation = iota
	ApplicationClose
	TUIRun
	SubjectCreate
	SubjectGet
	SubjectList
	SubjectUpdate
	SubjectDelete
)

type FailureCategory uint8

const UnexpectedFailure FailureCategory = iota

func (operation Operation) name() string {
	switch operation {
	case ApplicationStart:
		return "application_start"
	case ApplicationClose:
		return "application_close"
	case TUIRun:
		return "tui_run"
	case SubjectCreate:
		return "subject_create"
	case SubjectGet:
		return "subject_get"
	case SubjectList:
		return "subject_list"
	case SubjectUpdate:
		return "subject_update"
	case SubjectDelete:
		return "subject_delete"
	default:
		return "unknown"
	}
}

func (event MutationEvent) message() string {
	switch event {
	case SubjectCreated:
		return "subject created"
	case SubjectUpdated:
		return "subject updated"
	case SubjectDeleted:
		return "subject deleted"
	default:
		return "unknown mutation"
	}
}

func (category FailureCategory) name() string {
	// There is only one approved category; unknown values use the same fallback.
	return "unexpected_failure"
}

type Logger struct {
	logger *zerolog.Logger
}

func New(output io.Writer, application, level string) (Logger, error) {
	return newLogger(output, application, level, false)
}

func NewConsole(output io.Writer, application, level string) (Logger, error) {
	return newLogger(output, application, level, true)
}

func Nop() Logger {
	return Logger{}
}

// NewFile opens an append-only JSON log. Disabled logging creates no file.
// The caller must close the returned file when it is non-nil.
func NewFile(path, application, level string) (Logger, *os.File, error) {
	logger, err := New(io.Discard, application, level)
	if err != nil || logger.logger == nil {
		return logger, nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Logger{}, nil, fmt.Errorf("create log directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return Logger{}, nil, fmt.Errorf("open log: %w", err)
	}
	*logger.logger = logger.logger.Output(file)
	return logger, file, nil
}

func newLogger(output io.Writer, application, value string, console bool) (Logger, error) {
	level, err := parseLevel(value)
	if err != nil {
		return Logger{}, err
	}
	if level == zerolog.Disabled {
		return Nop(), nil
	}
	if console {
		output = zerolog.ConsoleWriter{Out: output, TimeFormat: time.RFC3339}
	}

	logger := zerolog.New(output).
		With().
		Timestamp().
		CallerWithSkipFrameCount(callerSkipFrameCount).
		Str("application", application).
		Logger().
		Level(level)
	return Logger{logger: &logger}, nil
}

func parseLevel(value string) (zerolog.Level, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return zerolog.InfoLevel, nil
	}
	level, err := zerolog.ParseLevel(value)
	if err != nil {
		return zerolog.NoLevel, fmt.Errorf("invalid THOUGHTS_LOG_LEVEL %q: %w", value, err)
	}
	return level, nil
}

func (l Logger) Started() {
	if l.logger == nil {
		return
	}
	l.logger.Info().Msg("starting application")
}

func (l Logger) Stopped() {
	if l.logger == nil {
		return
	}
	l.logger.Info().Msg("stopped application")
}

func (l Logger) Mutation(event MutationEvent, subjectID int64) {
	if l.logger == nil {
		return
	}
	l.logger.Info().Int64("subject_id", subjectID).Msg(event.message())
}

func (l Logger) Failure(operation Operation, category FailureCategory) {
	if l.logger == nil {
		return
	}
	l.logger.Error().Str("operation", operation.name()).Str("category", category.name()).Msg("operation failed")
}
