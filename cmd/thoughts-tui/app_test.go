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
			if view := model.View().Content; !strings.Contains(view, "Events") || strings.Contains(view, "local-user-id") {
				t.Fatalf("got view %q, want Events home without local user ID", view)
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
