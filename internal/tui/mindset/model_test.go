package mindset

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
)

type reader struct {
	items       []data.ThoughtSummaryView
	from, until time.Time
	err         error
}

func (s *reader) ListMindsetsInRange(_ context.Context, _ string, from, until time.Time) ([]data.ThoughtSummaryView, error) {
	s.from, s.until = from, until
	return s.items, s.err
}
func TestModel_Panel(t *testing.T) {
	t.Run("rotates in order and pauses while focused", func(t *testing.T) {
		service := &reader{items: []data.ThoughtSummaryView{{ThoughtID: 1, Preview: "first"}, {ThoughtID: 2, Preview: "second"}}}
		m := New(t.Context(), "u", service, logging.Nop())
		cmd := m.Load(time.Now())
		m, _ = m.Update(cmd())
		m, cmd = m.SetVisible(true)
		if cmd == nil {
			t.Fatal("missing rotation timer")
		}
		m, _ = m.Update(tickMsg{owner: m.owner, generation: m.generation})
		if !strings.Contains(m.View(), "second") {
			t.Fatalf("preview: got %q, want second", m.View())
		}
		m, _ = m.SetFocused(true)
		m, _ = m.Update(tickMsg{owner: m.owner, generation: m.generation})
		if !strings.Contains(m.View(), "second") {
			t.Fatal("focused preview rotated")
		}
	})
	t.Run("expands at preview and scrolls text only rows", func(t *testing.T) {
		service := &reader{items: []data.ThoughtSummaryView{{ThoughtID: 1, Preview: "one"}, {ThoughtID: 2, Preview: "two"}, {ThoughtID: 3, Preview: "three"}}}
		m := New(t.Context(), "u", service, logging.Nop())
		cmd := m.Load(time.Now())
		m, _ = m.Update(cmd())
		m.Resize(40, 2)
		m, _ = m.SetFocused(true)
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnd}))
		text := ansi.Strip(m.View())
		if strings.Contains(text, "one") || !strings.Contains(text, "three") || m.offset != 1 {
			t.Fatalf("scrolled panel: got %q offset=%d", text, m.offset)
		}
		_, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		if got := cmd().(OpenThoughtMsg).Thought.ThoughtID; got != 3 {
			t.Fatalf("opened ID: got %d, want 3", got)
		}
	})
	t.Run("rejects replies from the previous day", func(t *testing.T) {
		service := &reader{items: []data.ThoughtSummaryView{{ThoughtID: 1, Preview: "old"}}}
		m := New(t.Context(), "u", service, logging.Nop())
		old := m.Load(time.Now())()
		m.Load(time.Now().AddDate(0, 0, -1))
		m, _ = m.Update(old)
		if len(m.items) != 0 || !m.loading {
			t.Fatal("old reply replaced the new day")
		}
	})
	t.Run("uses calendar boundaries across daylight saving", func(t *testing.T) {
		service := &reader{}
		m := New(t.Context(), "u", service, logging.Nop())
		m.Load(time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC))()
		if got := service.until.Sub(service.from); got != 23*time.Hour {
			t.Fatalf("day duration: got %v, want 23h", got)
		}
	})
}

func TestModel_PanelFailuresAndTimers(t *testing.T) {
	t.Run("shows a safe failure and allows retry", func(t *testing.T) {
		service := &reader{err: errors.New("PRIVATE BODY")}
		m := New(t.Context(), "u", service, logging.Nop())
		cmd := m.Load(time.Now())
		m, _ = m.Update(cmd())
		if strings.Contains(m.View(), "PRIVATE") || !strings.Contains(m.View(), "r: retry") {
			t.Fatalf("error panel: got %q", m.View())
		}
		service.err = nil
		service.items = []data.ThoughtSummaryView{{ThoughtID: 1, Preview: "recovered"}}
		m, _ = m.SetFocused(true)
		m, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: 'r', Text: "r"}))
		m, _ = m.Update(cmd())
		if !strings.Contains(m.View(), "recovered") {
			t.Fatalf("retry: got %q", m.View())
		}
	})
	t.Run("hiding cancels the timer and queued ticks cannot rotate", func(t *testing.T) {
		service := &reader{items: []data.ThoughtSummaryView{{ThoughtID: 1, Preview: "one"}, {ThoughtID: 2, Preview: "two"}}}
		m := New(t.Context(), "u", service, logging.Nop())
		cmd := m.Load(time.Now())
		m, _ = m.Update(cmd())
		m, cmd = m.SetVisible(true)
		old := tickMsg{owner: m.owner, generation: m.generation}
		m, _ = m.SetVisible(false)
		if got := cmd(); got != nil {
			t.Fatalf("canceled timer: got %v, want nil", got)
		}
		m, _ = m.Update(old)
		if m.index != 0 {
			t.Fatal("hidden panel rotated")
		}
	})
	t.Run("expanded selection survives refresh by thought ID", func(t *testing.T) {
		service := &reader{items: []data.ThoughtSummaryView{{ThoughtID: 1, Preview: "one"}, {ThoughtID: 2, Preview: "two"}}}
		m := New(t.Context(), "u", service, logging.Nop())
		cmd := m.Load(time.Now())
		m, _ = m.Update(cmd())
		m.index = 1
		m.expanded = true
		service.items = []data.ThoughtSummaryView{{ThoughtID: 2, Preview: "two"}, {ThoughtID: 1, Preview: "one"}}
		cmd = m.Load(m.day)
		m, _ = m.Update(cmd())
		if m.index != 0 || !m.expanded {
			t.Fatalf("refreshed selection: got %d expanded=%v", m.index, m.expanded)
		}
	})
	t.Run("header uses a white background and black text", func(t *testing.T) {
		m := New(t.Context(), "u", &reader{}, logging.Nop())
		got := m.View()
		if !strings.Contains(got, "38;2;0;0;0") || !strings.Contains(got, "48;2;255;255;255") {
			t.Fatalf("header colors: got %q, want black on white", got)
		}
	})
}
