package tui

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
)

func TestModel_SubjectReadCancellation(t *testing.T) {
	t.Run("leaving Subjects cancels its running read", func(t *testing.T) {
		started := make(chan context.Context, 1)
		service := &subjectServiceStub{list: func(ctx context.Context, _ string) ([]data.Subject, error) {
			started <- ctx
			<-ctx.Done()
			return nil, ctx.Err()
		}}
		model := newSubjectTestModel(service)
		updated, cmd := model.openSubjects()
		model = updated.(Model)
		done := make(chan tea.Msg, 1)
		go func() { done <- cmd() }()
		ctx := <-started
		model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
		select {
		case got := <-done:
			if got != nil {
				t.Fatalf("got %T, want no cancelled reply", got)
			}
		case <-time.After(time.Second):
			t.Fatal("Subjects read survived navigation")
		}
		if ctx.Err() != context.Canceled {
			t.Fatalf("got %v, want cancelled read context", ctx.Err())
		}
	})
}

func TestModel_SubjectRequestOwnership(t *testing.T) {
	t.Run("reopening Subjects rejects an already queued reply from its previous opening", func(t *testing.T) {
		calls := 0
		service := &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) {
			calls++
			return []data.Subject{{SubjectID: int64(calls), SubjectName: "synthetic"}}, nil
		}}
		m := newSubjectTestModel(service)
		updated, old := m.openSubjects()
		m = updated.(Model)
		queued := old()
		updated, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
		m = updated.(Model)
		updated, current := m.openSubjects()
		m = updated.(Model)
		updated, _ = m.Update(queued)
		m = updated.(Model)
		if !m.subjects.loading {
			t.Fatal("obsolete reply completed the replacement request")
		}
		updated, _ = m.Update(current())
		m = updated.(Model)
		if m.subjects.loading || len(m.subjects.list.Items()) != 3 || m.subjects.list.Items()[2].(subjectRow).subject.SubjectID != 2 {
			t.Fatal("replacement Subjects request did not supply its own rows")
		}
	})
	t.Run("cancellation after listing skips remaining metrics calls", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		service := &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) { cancel(); return nil, nil }}
		m := newSubjectTestModel(service)
		m.ctx = ctx
		m.metrics = nil // Any subsequent call would panic instead of being skipped.
		cmd := m.listSubjects()
		if got := cmd(); got != nil {
			t.Fatalf("got %T, want no cancelled reply", got)
		}
	})
}

func TestModel_CancelledSessionReplies(t *testing.T) {
	t.Run("a queued failure cannot update or log after the UI session ends", func(t *testing.T) {
		var logs bytes.Buffer
		logger, err := logging.New(&logs, "test", "info")
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		service := &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) {
			return nil, errors.New("PRIVATE-FAILURE-MARKER")
		}}
		m := newSubjectTestModel(service)
		m.ctx, m.logger = ctx, logger
		updated, cmd := m.openSubjects()
		m = updated.(Model)
		queued := cmd()
		before := m.View().Content
		cancel()
		updated, next := m.Update(queued)
		if next != nil || updated.(Model).View().Content != before {
			t.Fatal("closed session applied a queued failure")
		}
		if logs.Len() != 0 {
			t.Fatal("closed session logged a queued failure")
		}
	})
}
