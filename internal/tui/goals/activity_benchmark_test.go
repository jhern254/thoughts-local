//go:build integration

package goals

import (
	"fmt"
	"os"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/data"
)

// Use a disposable migrated database loaded with scripts/demo_stress.sql.
// Compile first and measure without other builds or tests running. Setup and
// fixture reads stay outside the timed loops; no benchmark writes domain data.
func openActivityBenchmark(b *testing.B) *application.Runtime {
	b.Helper()
	dsn := os.Getenv("THOUGHTS_STRESS_DSN")
	if dsn == "" {
		b.Skip("set THOUGHTS_STRESS_DSN to a disposable stress-seeded database")
	}
	app, err := application.Open(b.Context(), dsn)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		if err := app.Close(); err != nil {
			b.Error(err)
		}
	})
	return app
}

func BenchmarkGoalActivityRead(b *testing.B) {
	app := openActivityBenchmark(b)
	reader := app.GoalProgress()
	first, err := reader.ListEarnedEvents(b.Context(), "demo", 1, data.GoalEventPageRequest{Limit: 80})
	if err != nil || len(first.Events) == 0 {
		b.Fatalf("fixture history: got %d Events, %v; want a populated stress Goal", len(first.Events), err)
	}
	cursor := first.Events[len(first.Events)-1].Cursor()
	older, err := reader.ListEarnedEvents(b.Context(), "demo", 1, data.GoalEventPageRequest{Limit: 80, Cursor: &cursor})
	if err != nil || len(older.Events) == 0 {
		b.Fatalf("older fixture window: got %d Events, %v", len(older.Events), err)
	}
	newerCursor := older.Events[0].Cursor()
	for _, tc := range []struct {
		name    string
		goalID  int64
		request data.GoalEventPageRequest
	}{
		{"FirstWindow", 1, data.GoalEventPageRequest{Limit: 80}},
		{"OlderWindow", 1, data.GoalEventPageRequest{Limit: 80, Cursor: &cursor}},
		{"NewerWindow", 1, data.GoalEventPageRequest{Limit: 80, Cursor: &newerCursor, Direction: data.EarnedEventsNewer}},
		{"MaximumWindow", 1, data.GoalEventPageRequest{Limit: data.MaxGoalEventPageSize}},
		{"EmptyGoal", 48, data.GoalEventPageRequest{Limit: 80}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := reader.ListEarnedEvents(b.Context(), "demo", tc.goalID, tc.request); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
	b.Run("ThoughtExpansion", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := reader.ListEventLineageThoughts(b.Context(), "demo", 1, first.Events[0].EventID, data.ThoughtSummaryViewRequest{}); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkGoalActivityPresentation(b *testing.B) {
	app := openActivityBenchmark(b)
	for _, size := range [][2]int{{100, 35}, {48, 24}} {
		for _, mode := range []string{"View", "Navigate", "Expand"} {
			b.Run(fmt.Sprintf("%dx%d/%s", size[0], size[1], mode), func(b *testing.B) {
				m := New(b.Context(), "demo", app.Goals(), app.GoalAssignments(), app.SubjectGoals(), app.Subjects(), app.GoalProgress())
				m.open = true
				m.screen = detailView
				m.Resize(size[0], size[1])
				m, cmd := m.Update(m.loadDetail(1)())
				if cmd == nil {
					b.Fatal("missing initial history read")
				}
				m, _ = m.Update(activityReadReply(cmd))
				b.Cleanup(m.Close)
				if len(m.activity.events.Events) < 2 {
					b.Fatal("fixture must contain multiple earned Events")
				}
				eventID := m.activity.events.Events[0].EventID
				down := true
				b.ReportAllocs()
				for b.Loop() {
					switch mode {
					case "Navigate":
						code := tea.KeyDown
						if !down {
							code = tea.KeyUp
						}
						down = !down
						m, cmd = m.Update(tea.KeyPressMsg{Code: code})
						if cmd != nil {
							b.Fatal("local navigation unexpectedly started a read")
						}
					case "Expand":
						m, _ = m.Update(activityReadReply(m.loadActivityThoughts(eventID, data.ThoughtSummaryViewRequest{})))
					}
					_ = m.View()
					if mode == "Expand" {
						m.collapseActivity()
					}
				}
			})
		}
	}
}
