//go:build integration

package integration_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/goal"
)

// A small earned session lets browser acceptance exercise lineage through real
// services. The ordinary demo's raw SQL Events predate automatic contribution
// creation, so they deliberately do not acquire credit merely by being read.
func seedGoalLineageBrowser(t *testing.T, runtime *application.Runtime) {
	t.Helper()
	seedLongGoalHistoryBrowser(t, runtime)
	user := runtime.LocalUser().UserID
	programming, err := runtime.Subjects().Create(t.Context(), user, "Lineage Programming")
	if err != nil {
		t.Fatal(err)
	}
	research, err := runtime.Subjects().Create(t.Context(), user, "Lineage Research")
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.GoalAssignments().Create(t.Context(), user, goal.CreateGoalInput{Name: "Explore earned activity", TargetSeconds: 3600, Priority: "high"}, []int64{programming.SubjectID, research.SubjectID})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().UTC().Truncate(time.Second).Add(-8 * time.Hour)
	_, err = runtime.Events().CreatePast(t.Context(), user, "Implement Goal lineage", start, start.Add(30*time.Minute), []int64{programming.SubjectID, research.SubjectID})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		text      string
		subjectID *int64
		offset    time.Duration
	}{
		{"Lineage: Events own time.", &programming.SubjectID, time.Minute},
		{"Lineage: one Event can reach several Goals.", &research.SubjectID, 2 * time.Minute},
		{"Excluded Misc lineage Thought", nil, 3 * time.Minute},
		{"Excluded outside lineage Thought", &programming.SubjectID, 31 * time.Minute},
	} {
		if _, err := runtime.Thoughts().Create(t.Context(), user, entry.text, entry.subjectID, start.Add(entry.offset)); err != nil {
			t.Fatal(err)
		}
	}
}

// A separate Goal provides more than ten display pages without changing the
// Thought-opening fixture above. Equal timestamps also exercise the ID tie-break.
func seedLongGoalHistoryBrowser(t *testing.T, runtime *application.Runtime) {
	t.Helper()
	user := runtime.LocalUser().UserID
	subject, err := runtime.Subjects().Create(t.Context(), user, "Paging history")
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.GoalAssignments().Create(t.Context(), user, goal.CreateGoalInput{Name: "Browse long history", TargetSeconds: 3600}, []int64{subject.SubjectID})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 125; i++ {
		_, err = runtime.Events().CreatePast(t.Context(), user, fmt.Sprintf("History entry %03d", i), time.Unix(100, 0), time.Unix(100, 0), []int64{subject.SubjectID})
		if err != nil {
			t.Fatal(err)
		}
	}
}
