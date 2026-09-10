package cliutil

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	cli "github.com/urfave/cli/v3"
)

func TestDiagnostics_Command(t *testing.T) {
	const private = "PRIVATE-DIAGNOSTIC-MARKER"
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"root usage", []string{"test", "--" + private}},
		{"child usage", []string{"test", "child", "--" + private}},
	} {
		t.Run(tc.name+" returns original error without leaking arguments", func(t *testing.T) {
			var output, diagnostic bytes.Buffer
			message := ""
			command := &cli.Command{Name: "test", Writer: &output, ErrWriter: &diagnostic, Commands: []*cli.Command{{Name: "child"}}}
			ConfigureDiagnostics(command, &message)
			err := command.Run(context.Background(), tc.args)
			if err == nil || !strings.Contains(err.Error(), private) {
				t.Fatalf("got error %v, want original parser error", err)
			}
			if got, want := message, "Invalid command arguments. Use --help for usage."; got != want {
				t.Fatalf("got message %q, want %q", got, want)
			}
			if strings.Contains(output.String()+diagnostic.String(), private) {
				t.Fatal("got private argument in output, want safe framework output")
			}
		})
	}
}

func TestExitCode(t *testing.T) {
	const private = "PRIVATE-DIAGNOSTIC-MARKER"
	for _, tc := range []struct {
		name          string
		action, close error
		want          int
	}{
		{"ordinary failure", errors.New(private), nil, 1},
		{"explicit exit", cli.Exit(private, 3), nil, 3},
		{"aggregate retains action exit", cli.Exit(private, 7), errors.New(private), 7},
		{"aggregate uses last explicit exit", cli.Exit(private, 7), cli.Exit(private, 9), 9},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			message := ""
			command := &cli.Command{Name: "test", Writer: &output,
				Action: func(context.Context, *cli.Command) error { return tc.action },
				After:  func(context.Context, *cli.Command) error { return tc.close },
			}
			ConfigureDiagnostics(command, &message)
			err := command.Run(context.Background(), []string{"test"})
			if got := ExitCode(err); got != tc.want {
				t.Fatalf("got exit code %d, want %d", got, tc.want)
			}
			if output.Len() != 0 {
				t.Fatalf("got output %q, want no framework diagnostics", output.String())
			}
		})
	}
}
