package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/event"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/metrics"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/jhern254/go-thoughts/internal/timeline"
)

type runtimeStub struct {
	events       *event.Service
	timelineView *timeline.Service
	localUser    *data.User
	subjects     *subject.Service
	close        func() error
}

func (stub *runtimeStub) Thoughts() *thought.Service { return nil }
func (stub *runtimeStub) Metrics() *metrics.Service  { return nil }

func (stub *runtimeStub) Events() *event.Service          { return stub.events }
func (stub *runtimeStub) TimelineView() *timeline.Service { return stub.timelineView }

func (stub *runtimeStub) LocalUser() *data.User {
	return stub.localUser
}

func (stub *runtimeStub) Subjects() *subject.Service {
	return stub.subjects
}

func (stub *runtimeStub) Close() error {
	if stub.close == nil {
		return nil
	}
	return stub.close()
}

func TestTUI_DatabaseDSNPrecedence(t *testing.T) {
	tests := []struct {
		name    string
		envDSN  string
		args    []string
		wantDSN string
	}{
		{name: "uses default", args: []string{"thoughts-tui"}, wantDSN: defaultSQLiteDSN},
		{name: "uses environment variable", envDSN: "file:environment.db", args: []string{"thoughts-tui"}, wantDSN: "file:environment.db"},
		{name: "flag overrides environment variable", envDSN: "file:environment.db", args: []string{"thoughts-tui", "--db-dsn", "file:flag.db"}, wantDSN: "file:flag.db"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("THOUGHTS_DB_DSN", tt.envDSN)
			want := errors.New("stop after resolving DSN")
			var gotDSN string
			app := newApplication(strings.NewReader(""), io.Discard, io.Discard, logging.Nop())
			app.openRuntime = func(_ context.Context, dsn string) (runtime, error) {
				gotDSN = dsn
				return nil, want
			}

			err := newTUI(app).Run(context.Background(), tt.args)

			if err != want {
				t.Fatalf("got error %v, want %v", err, want)
			}
			if gotDSN != tt.wantDSN {
				t.Fatalf("got DSN %q, want %q", gotDSN, tt.wantDSN)
			}
		})
	}
}

func TestTUI_RuntimeLifecycle(t *testing.T) {
	t.Run("launches Events home after bootstrap and closes runtime", func(t *testing.T) {
		closeCalls := 0
		programCalls := 0
		app := newApplication(strings.NewReader(""), io.Discard, io.Discard, logging.Nop())
		app.openRuntime = func(context.Context, string) (runtime, error) {
			return &runtimeStub{
				localUser: &data.User{UserID: "local-user-id"},
				close: func() error {
					closeCalls++
					return nil
				},
			}, nil
		}
		app.runProgram = func(_ context.Context, model tea.Model, _ io.Reader, _ io.Writer) error {
			programCalls++
			if view := model.View().Content; !strings.Contains(view, "Events") || !strings.HasPrefix(view, "Local user: local-user-id\n") {
				t.Fatalf("got view %q, want local user above Events home", view)
			}
			return nil
		}

		err := newTUI(app).Run(context.Background(), []string{"thoughts-tui"})

		if err != nil {
			t.Fatal(err)
		}
		if programCalls != 1 || closeCalls != 1 {
			t.Fatalf("got %d program calls and %d close calls, want 1 each", programCalls, closeCalls)
		}
	})

	t.Run("does not launch after runtime open failure", func(t *testing.T) {
		want := errors.New("open failed")
		app := newApplication(strings.NewReader(""), io.Discard, io.Discard, logging.Nop())
		app.openRuntime = func(context.Context, string) (runtime, error) {
			return nil, want
		}
		app.runProgram = func(context.Context, tea.Model, io.Reader, io.Writer) error {
			t.Fatal("launched program after runtime open failure")
			return nil
		}

		err := newTUI(app).Run(context.Background(), []string{"thoughts-tui"})

		if err != want {
			t.Fatalf("got error %v, want %v", err, want)
		}
	})

	t.Run("closes runtime after program failure", func(t *testing.T) {
		want := errors.New("program failed")
		closeCalls := 0
		app := newApplication(strings.NewReader(""), io.Discard, io.Discard, logging.Nop())
		app.openRuntime = func(context.Context, string) (runtime, error) {
			return &runtimeStub{
				localUser: &data.User{UserID: "local-user-id"},
				close: func() error {
					closeCalls++
					return nil
				},
			}, nil
		}
		app.runProgram = func(context.Context, tea.Model, io.Reader, io.Writer) error {
			return want
		}

		err := newTUI(app).Run(context.Background(), []string{"thoughts-tui"})

		if err != want {
			t.Fatalf("got error %v, want %v", err, want)
		}
		if closeCalls != 1 {
			t.Fatalf("closed runtime %d times, want 1", closeCalls)
		}
	})

	t.Run("returns runtime close error", func(t *testing.T) {
		want := errors.New("close failed")
		app := newApplication(strings.NewReader(""), io.Discard, io.Discard, logging.Nop())
		app.openRuntime = func(context.Context, string) (runtime, error) {
			return &runtimeStub{
				localUser: &data.User{UserID: "local-user-id"},
				close: func() error {
					return want
				},
			}, nil
		}
		app.runProgram = func(context.Context, tea.Model, io.Reader, io.Writer) error {
			return nil
		}

		err := newTUI(app).Run(context.Background(), []string{"thoughts-tui"})

		if err != want {
			t.Fatalf("got error %v, want %v", err, want)
		}
	})
}

func TestTUI_BrowserMode(t *testing.T) {
	t.Run("selects browser explicitly and closes the shared runtime after serving", func(t *testing.T) {
		t.Setenv("THOUGHTS_BROWSER_PORT", "8123")
		closed := false
		app := newApplication(strings.NewReader(""), io.Discard, io.Discard, logging.Nop())
		app.openRuntime = func(context.Context, string) (runtime, error) {
			return &runtimeStub{localUser: &data.User{UserID: "browser-user"}, close: func() error { closed = true; return nil }}, nil
		}
		app.runProgram = func(context.Context, tea.Model, io.Reader, io.Writer) error {
			t.Fatal("native program launched")
			return nil
		}
		app.runBrowser = func(ctx context.Context, port int, factory func(context.Context) tea.Model, out io.Writer, logger logging.Logger) error {
			if port != 8124 {
				t.Fatalf("port got %d, want flag override 8124", port)
			}
			if closed {
				t.Fatal("runtime closed while serving")
			}
			first := factory(ctx)
			first, _ = first.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
			first, _ = first.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
			second := factory(ctx)
			if !strings.Contains(second.View().Content, "Events") || !strings.Contains(second.View().Content, "browser-user") {
				t.Fatal("new session did not start on Events with bootstrapped user")
			}
			return nil
		}
		if err := newTUI(app).Run(context.Background(), []string{"thoughts-tui", "--browser", "--browser-port", "8124"}); err != nil {
			t.Fatal(err)
		}
		if !closed {
			t.Fatal("runtime was not closed")
		}
	})
	t.Run("validates browser configuration before opening runtime", func(t *testing.T) {
		app := newApplication(strings.NewReader(""), io.Discard, io.Discard, logging.Nop())
		app.openRuntime = func(context.Context, string) (runtime, error) {
			t.Fatal("opened runtime for invalid port")
			return nil, nil
		}
		if err := newTUI(app).Run(context.Background(), []string{"thoughts-tui", "--browser", "--browser-port", "65536"}); err == nil {
			t.Fatal("accepted invalid port")
		}
	})
	t.Run("runtime failure prevents browser startup", func(t *testing.T) {
		want := errors.New("runtime failure")
		app := newApplication(strings.NewReader(""), io.Discard, io.Discard, logging.Nop())
		app.openRuntime = func(context.Context, string) (runtime, error) { return nil, want }
		app.runBrowser = func(context.Context, int, func(context.Context) tea.Model, io.Writer, logging.Logger) error {
			t.Fatal("served after runtime failure")
			return nil
		}
		if err := newTUI(app).Run(context.Background(), []string{"thoughts-tui", "--browser"}); !errors.Is(err, want) {
			t.Fatalf("got %v, want runtime failure", err)
		}
	})
	t.Run("server failure closes runtime and reports a safe diagnostic", func(t *testing.T) {
		closed := false
		want := errors.New("PRIVATE-BROWSER-ERROR")
		app := newApplication(strings.NewReader(""), io.Discard, io.Discard, logging.Nop())
		app.openRuntime = func(context.Context, string) (runtime, error) {
			return &runtimeStub{close: func() error { closed = true; return nil }}, nil
		}
		app.runBrowser = func(context.Context, int, func(context.Context) tea.Model, io.Writer, logging.Logger) error {
			return want
		}
		err := newTUI(app).Run(context.Background(), []string{"thoughts-tui", "--browser"})
		if !errors.Is(err, want) || !closed {
			t.Fatalf("got %v, closed %v; want original failure and cleanup", err, closed)
		}
		if app.failureMessage != "Could not run the browser interface." {
			t.Fatalf("got diagnostic %q", app.failureMessage)
		}
	})
}
