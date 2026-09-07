package logging

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

const callerSkipFrameCount = 3

type Level struct {
	value zerolog.Level
}

func ParseLevel(value string) (Level, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return Level{value: zerolog.InfoLevel}, nil
	}
	level, err := zerolog.ParseLevel(value)
	if err != nil {
		return Level{}, fmt.Errorf("invalid THOUGHTS_LOG_LEVEL %q: %w", value, err)
	}
	return Level{value: level}, nil
}

func (l Level) Disabled() bool {
	return l.value == zerolog.Disabled
}

type Field struct {
	key   string
	value any
}

func String(key, value string) Field {
	return Field{key: key, value: value}
}

func Int64(key string, value int64) Field {
	return Field{key: key, value: value}
}

type Logger struct {
	logger zerolog.Logger
}

func New(output io.Writer, application string, level Level) Logger {
	return newLogger(output, application, level)
}

func NewConsole(output io.Writer, application string, level Level) Logger {
	console := zerolog.ConsoleWriter{Out: output, TimeFormat: time.RFC3339}
	return newLogger(console, application, level)
}

func Nop() Logger {
	return Logger{logger: zerolog.Nop()}
}

func newLogger(output io.Writer, application string, level Level) Logger {
	logger := zerolog.New(output).
		With().
		Timestamp().
		CallerWithSkipFrameCount(callerSkipFrameCount).
		Str("application", application).
		Logger().
		Level(level.value)
	return Logger{logger: logger}
}

func (l Logger) Info(message string, fields ...Field) {
	addFields(l.logger.Info(), fields).Msg(message)
}

func (l Logger) Debug(message string, fields ...Field) {
	addFields(l.logger.Debug(), fields).Msg(message)
}

func (l Logger) Error(err error, message string, fields ...Field) {
	addFields(l.logger.Error().Err(err), fields).Msg(message)
}

func addFields(event *zerolog.Event, fields []Field) *zerolog.Event {
	for _, field := range fields {
		event = event.Interface(field.key, field.value)
	}
	return event
}
