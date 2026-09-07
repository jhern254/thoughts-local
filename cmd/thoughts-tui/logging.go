package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"
)

const (
	tuiApplicationName = "thoughts-tui"
	defaultTUILogPath  = "data/thoughts-tui.log"
)

func newLogger(path, value string) (zerolog.Logger, *os.File, error) {
	level, err := logLevel(value)
	if err != nil {
		return zerolog.Logger{}, nil, err
	}
	if level == zerolog.Disabled {
		return zerolog.Nop(), nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return zerolog.Logger{}, nil, fmt.Errorf("create TUI log directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return zerolog.Logger{}, nil, fmt.Errorf("open TUI log: %w", err)
	}

	logger := zerolog.New(file).
		With().
		Timestamp().
		Caller().
		Str("application", tuiApplicationName).
		Logger().
		Level(level)
	return logger, file, nil
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
