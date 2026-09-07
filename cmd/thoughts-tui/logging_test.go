package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTUILogging_UsesPrivateJSONFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "thoughts.log")
	logger, file, err := newLogger(path, "")
	if err != nil {
		t.Fatal(err)
	}
	var tuiOutput bytes.Buffer

	logger.Info("starting application")
	tuiOutput.WriteString("screen")
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(contents); !strings.Contains(got, `"message":"starting application"`) {
		t.Fatalf("got log file %q", got)
	}
	if got, want := tuiOutput.String(), "screen"; got != want {
		t.Fatalf("got TUI output %q, want %q", got, want)
	}
}
