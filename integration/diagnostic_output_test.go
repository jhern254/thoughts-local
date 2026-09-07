//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDiagnosticOutput_Executables(t *testing.T) {
	const private = "PRIVATE-DIAGNOSTIC-MARKER"
	t.Setenv("URFAVE_CLI_TRACING", "")
	t.Setenv("TEA_TRACE", "")
	t.Setenv("CLI_TEMPLATE_ERROR_DEBUG", "")
	t.Setenv("THOUGHTS_LOG_LEVEL", "disabled")
	t.Setenv("THOUGHTS_DB_DSN", "file:"+private+"?mode=invalid")
	for _, adapter := range []string{"thoughts", "thoughts-tui", "api"} {
		t.Run(adapter, func(t *testing.T) {
			binary := filepath.Join(t.TempDir(), adapter)
			build := exec.Command("go", "build", "-o", binary, "../cmd/"+adapter)
			if out, err := build.CombinedOutput(); err != nil {
				t.Fatalf("got build error %v (%s), want nil", err, out)
			}
			type diagnosticCase struct {
				name        string
				args        []string
				level       string
				wantCode    int
				wantMessage string
			}
			cases := []diagnosticCase{
				{"invalid flags", []string{"--" + private}, "disabled", 1, "Invalid command arguments."},
				{"help hides environment defaults", []string{"--help"}, "disabled", 0, ""},
				{"database initialization", nil, "disabled", 1, "Could not open the application database."},
			}
			if adapter == "api" {
				cases[0].wantCode = 2
			} else {
				cases[2].wantMessage = "Could not start the application."
				cases = append(cases,
					diagnosticCase{"logger initialization", nil, private, 1, "Could not initialize application logging."},
					diagnosticCase{"unknown help topic", []string{"--help", private}, "disabled", 3, ""},
					diagnosticCase{"help command parser", []string{"help", "--" + private}, "disabled", 1, ""},
				)
			}
			if adapter == "thoughts-tui" {
				cases = append(cases, diagnosticCase{"log file initialization", nil, "info", 1, "Could not initialize application logging."})
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Setenv("THOUGHTS_LOG_LEVEL", tc.level)
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancel()
					command := exec.CommandContext(ctx, binary, tc.args...)
					command.Dir = t.TempDir()
					if tc.name == "log file initialization" {
						// Prevent the logger from creating its data directory.
						if err := os.WriteFile(filepath.Join(command.Dir, "data"), []byte(private), 0600); err != nil {
							t.Fatal(err)
						}
					}
					var stdout, stderr bytes.Buffer
					command.Stdout = &stdout
					command.Stderr = &stderr
					err := command.Run()
					if ctx.Err() != nil {
						t.Fatalf("got timeout %v, want process exit", ctx.Err())
					}
					gotCode := 0
					if err != nil {
						var exitErr *exec.ExitError
						if !errors.As(err, &exitErr) {
							t.Fatalf("got process error %v, want exit status", err)
						}
						gotCode = exitErr.ExitCode()
					}
					if gotCode != tc.wantCode {
						t.Fatalf("got exit code %d, want %d; stdout=%q stderr=%q", gotCode, tc.wantCode, stdout.String(), stderr.String())
					}
					// These invocations request help or fail before producing application data.
					for stream, output := range map[string]string{"stdout": stdout.String(), "stderr": stderr.String()} {
						if strings.Contains(output, private) {
							t.Fatalf("got private marker in %s, want safe diagnostic/help", stream)
						}
					}
					if tc.wantMessage != "" && !strings.Contains(stdout.String()+stderr.String(), tc.wantMessage) {
						t.Fatalf("got stdout=%q stderr=%q, want %q", stdout.String(), stderr.String(), tc.wantMessage)
					}
					if tc.name == "help hides environment defaults" && !strings.Contains(strings.ToLower(stdout.String()+stderr.String()), "usage") {
						t.Fatal("got no usage output, want help")
					}
					if contents, err := os.ReadFile(filepath.Join(command.Dir, "data", "thoughts-tui.log")); err == nil && strings.Contains(string(contents), private) {
						t.Fatal("got private marker in log file, want safe events")
					}
				})
			}
		})
	}
}
