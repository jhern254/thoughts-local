package logging

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

const callerSkipFrameCount = 3

type Fields map[string]any

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

func Disabled(value string) (bool, error) {
	level, err := parseLevel(value)
	return level == zerolog.Disabled, err
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

func (l Logger) Info(message string, fields ...Fields) {
	if l.logger == nil {
		return
	}
	addFields(l.logger.Info(), fields).Msg(message)
}

func (l Logger) Error(err error, message string, fields ...Fields) {
	if l.logger == nil {
		return
	}
	addFields(l.logger.Error().Err(err), fields).Msg(message)
}

func addFields(event *zerolog.Event, fields []Fields) *zerolog.Event {
	for _, values := range fields {
		event = event.Fields(map[string]any(values))
	}
	return event
}
