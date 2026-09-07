package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
)

func TestCLI_Logging(t *testing.T) {
	var logs, output bytes.Buffer
	logger, err := logging.NewConsole(&logs, "thoughts-cli", "")
	if err != nil {
		t.Fatal(err)
	}
	app := newApplication(&output, io.Discard, logger)
	app.openRuntime = func(context.Context, string) (cliRuntime, error) {
		return &cliRuntimeStub{localUser: &data.User{UserID: "local-user"}}, nil
	}

	err = newCLI(app).Run(context.Background(), []string{"thoughts", "subjects", "create"})

	if err == nil {
		t.Fatal("expected argument error")
	}
	for _, want := range []string{"starting application", "stopped application"} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("logs %q do not contain %q", logs.String(), want)
		}
	}
	if output.Len() != 0 {
		t.Fatalf("got command output %q", output.String())
	}
}

func newTestLogger(t *testing.T, output io.Writer) logging.Logger {
	t.Helper()
	logger, err := logging.New(output, "test", "info")
	if err != nil {
		t.Fatal(err)
	}
	return logger
}
