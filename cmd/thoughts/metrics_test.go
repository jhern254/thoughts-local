package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
)

type metricsStub struct {
	rows  []data.SubjectThoughtCount
	err   error
	calls int
}

func (s *metricsStub) ThoughtCountsBySubject(_ context.Context, userID string) ([]data.SubjectThoughtCount, error) {
	if userID != "u" {
		panic("wrong metric scope")
	}
	s.calls++
	return s.rows, s.err
}

func TestMetricsCommand_ThoughtCounts(t *testing.T) {
	t.Run("writes only tab separated counts to command output", func(t *testing.T) {
		var out, logs bytes.Buffer
		service := &metricsStub{rows: []data.SubjectThoughtCount{{SubjectID: 1, Count: 0}, {SubjectID: 2, Count: 3}}}
		app := &application{metrics: service, userID: "u", out: &out, logger: newTestLogger(t, &logs)}
		if err := newMetricsCommand(app).Run(context.Background(), []string{"metrics", "thought-counts"}); err != nil {
			t.Fatal(err)
		}
		if out.String() != "1\t0\n2\t3\n" || logs.Len() != 0 || service.calls != 1 {
			t.Fatalf("got output %q, logs %q, calls %d", &out, &logs, service.calls)
		}
		out.Reset()
		service.rows = nil
		if err := newMetricsCommand(app).Run(context.Background(), []string{"metrics", "thought-counts"}); err != nil || out.Len() != 0 {
			t.Fatalf("empty counts produced %q, error %v", &out, err)
		}
	})
	t.Run("rejects arguments without querying", func(t *testing.T) {
		service := &metricsStub{}
		app := &application{metrics: service}
		if err := newMetricsCommand(app).Run(context.Background(), []string{"metrics", "thought-counts", "extra"}); err == nil || service.calls != 0 {
			t.Fatalf("got error %v, calls %d", err, service.calls)
		}
	})
	t.Run("preserves original failures while logging only approved fields", func(t *testing.T) {
		for _, tt := range []struct {
			err      error
			category string
		}{
			{errors.New("private-marker"), "unexpected_failure"},
			{fmt.Errorf("private-marker: %w", data.ErrDatabaseBusy), "database_busy"},
		} {
			var out, logs bytes.Buffer
			app := &application{metrics: &metricsStub{err: tt.err}, userID: "u", out: &out, logger: newTestLogger(t, &logs)}
			err := newMetricsCommand(app).Run(context.Background(), []string{"metrics", "thought-counts"})
			if !errors.Is(err, tt.err) || out.Len() != 0 {
				t.Fatalf("got error %v and output %q", err, &out)
			}
			var event map[string]any
			if err := json.Unmarshal(logs.Bytes(), &event); err != nil {
				t.Fatal(err)
			}
			if len(event) != 7 || event["level"] != "error" || event["operation"] != "thought_counts_by_subject" || event["category"] != tt.category || event["message"] != "operation failed" || event["application"] != "test" || event["caller"] == nil || event["time"] == nil {
				t.Fatalf("unexpected fields: %v", event)
			}
			if strings.Contains(logs.String()+app.failureMessage, "private-marker") {
				t.Fatal("private error escaped")
			}
		}
	})
}
