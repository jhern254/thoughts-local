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
	disabled, err := logging.Disabled(value)
	if err != nil {
		return logging.Logger{}, nil, err
	}
	if disabled {
		return logging.Nop(), nil, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return logging.Logger{}, nil, fmt.Errorf("create TUI log directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return logging.Logger{}, nil, fmt.Errorf("open TUI log: %w", err)
	}

	logger, err := logging.New(file, tuiApplicationName, value)
	if err != nil {
		if closeErr := file.Close(); closeErr != nil {
			return logging.Logger{}, nil, fmt.Errorf("configure TUI log: %v; close TUI log: %w", err, closeErr)
		}
		return logging.Logger{}, nil, err
	}
	return logger, file, nil
}
