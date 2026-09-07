package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/jhern254/go-thoughts/internal/logging"
)

const (
	tuiApplicationName = "thoughts-tui"
	defaultTUILogPath  = "data/thoughts-tui.log"
)

func newLogger(path, value string) (logging.Logger, *os.File, error) {
	level, err := logging.ParseLevel(value)
	if err != nil {
		return logging.Logger{}, nil, err
	}
	if level.Disabled() {
		return logging.Nop(), nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return logging.Logger{}, nil, fmt.Errorf("create TUI log directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return logging.Logger{}, nil, fmt.Errorf("open TUI log: %w", err)
	}

	return logging.New(file, tuiApplicationName, level), file, nil
}
