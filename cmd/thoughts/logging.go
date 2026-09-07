package main

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

const cliApplicationName = "thoughts-cli"

func newLogger(output io.Writer, value string) (zerolog.Logger, error) {
	level, err := logLevel(value)
	if err != nil {
		return zerolog.Logger{}, err
	}
	if level == zerolog.Disabled {
		return zerolog.Nop(), nil
	}

	console := zerolog.ConsoleWriter{Out: output, TimeFormat: time.RFC3339}
	return zerolog.New(console).
		With().
		Timestamp().
		Caller().
		Str("application", cliApplicationName).
		Logger().
		Level(level), nil
}

func logLevel(value string) (zerolog.Level, error) {
	if strings.TrimSpace(value) == "" {
		return zerolog.InfoLevel, nil
	}
	level, err := zerolog.ParseLevel(value)
	if err != nil {
		return zerolog.NoLevel, fmt.Errorf("invalid THOUGHTS_LOG_LEVEL %q: %w", value, err)
	}
	return level, nil
}
