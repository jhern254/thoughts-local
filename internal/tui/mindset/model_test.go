package mindset

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
		if !m.Expanded() {
			t.Fatal("focusing must expand the scrolling list")
		}
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnd}))
		text := ansi.Strip(m.View())
		if strings.Contains(text, "one") || !strings.Contains(text, "three") || m.offset != 1 {
			t.Fatalf("scrolled panel: got %q offset=%d", text, m.offset)
		}
		_, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		if got := cmd().(OpenThoughtMsg).Thought.ThoughtID; got != 3 {
			t.Fatalf("opened ID: got %d, want 3", got)
		}
		m, _ = m.SetFocused(false)
		if m.Expanded() || strings.Contains(m.View(), "\n") || !strings.Contains(m.View(), "three") {
			t.Fatalf("blurred panel: got %q expanded=%v, want inline selected preview", m.View(), m.Expanded())
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
		m, _ = m.SetFocused(true)
		service.items = []data.ThoughtSummaryView{{ThoughtID: 2, Preview: "two"}, {ThoughtID: 1, Preview: "one"}}
		cmd = m.Load(m.day)
		m, _ = m.Update(cmd())
		if m.index != 0 || !m.Expanded() {
			t.Fatalf("refreshed selection: got %d expanded=%v", m.index, m.Expanded())
		}
	})
	t.Run("header uses black text on a white block", func(t *testing.T) {
		m := New(t.Context(), "u", &reader{}, logging.Nop())
		want := lipgloss.NewStyle().Background(color.White).Foreground(color.Black).Padding(0, 1).Render("Mindset") + " ..."
		if got := m.View(); got != want {
			t.Fatalf("empty header: got %q, want %q", got, want)
		}
	})
}

func TestModel_InlinePreview(t *testing.T) {
	for _, tc := range []struct {
		name  string
		items []data.ThoughtSummaryView
		want  string
	}{
		{name: "empty", want: " Mindset  ..."},
		{name: "populated", items: []data.ThoughtSummaryView{{ThoughtID: 1, Preview: "Keep learning"}}, want: " Mindset  Keep learning"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New(t.Context(), "u", &reader{items: tc.items}, logging.Nop())
			m, _ = m.Update(m.Load(time.Now())())
			if got := ansi.Strip(m.View()); got != tc.want {
				t.Fatalf("collapsed row: got %q, want %q", got, tc.want)
			}
			if got := m.Height(); got != 1 {
				t.Fatalf("collapsed height: got %d, want 1", got)
			}
		})
	}
	t.Run("clips Unicode preview to the remaining header width", func(t *testing.T) {
		m := New(t.Context(), "u", &reader{items: []data.ThoughtSummaryView{{ThoughtID: 1, Preview: "界界界界界界界界"}}}, logging.Nop())
		m, _ = m.Update(m.Load(time.Now())())
		m.Resize(18, 5)
		got := ansi.Strip(m.View())
		if !strings.HasPrefix(got, " Mindset  界") || !strings.HasSuffix(got, "…") || ansi.StringWidth(got) > 18 {
			t.Fatalf("clipped row: got %q, want inline Unicode within 18 cells", got)
		}
	})
}

func TestModel_EmptyFocus(t *testing.T) {
	m := New(t.Context(), "u", &reader{}, logging.Nop())
	m, _ = m.Update(m.Load(time.Now())())
	m, _ = m.SetFocused(true)
	got := m.View()
	if text := ansi.Strip(got); text != " Mindset │..." {
		t.Fatalf("focused empty row: got %q, want title and selected ellipsis", text)
	}
	if !strings.Contains(got, "38;2;238;111;248") || !strings.Contains(got, "38;2;173;88;180") {
		t.Fatalf("focused empty style: got %q, want purple picker text and rail", got)
	}
	m, _ = m.SetFocused(false)
	if got := ansi.Strip(m.View()); got != " Mindset  ..." {
		t.Fatalf("blurred empty row: got %q, want unselected ellipsis", got)
	}
}

func TestModel_ExpandedAlignment(t *testing.T) {
	for _, width := range []int{80, 24} {
		t.Run(fmt.Sprintf("width %d", width), func(t *testing.T) {
			m := New(t.Context(), "u", &reader{items: []data.ThoughtSummaryView{
				{ThoughtID: 1, Preview: "First 界 thought"},
				{ThoughtID: 2, Preview: "Second 界 thought with a long continuation"},
			}}, logging.Nop())
			m, _ = m.Update(m.Load(time.Now())())
			m.Resize(width, 5)
			collapsed := ansi.Strip(m.View())
			column := ansi.StringWidth(strings.Split(collapsed, "First")[0])
			m, _ = m.SetFocused(true)
			rows := strings.Split(ansi.Strip(m.View()), "\n")
			if len(rows) != 2 || m.Height() != 2 {
				t.Fatalf("expanded rows: got %q, height %d, want two rows", rows, m.Height())
			}
			for i, word := range []string{"First", "Second"} {
				position := strings.Index(rows[i], word)
				if position < 0 || ansi.StringWidth(rows[i][:position]) != column || ansi.StringWidth(rows[i]) > width {
					t.Fatalf("picker row %d: got %q, want text at column %d within %d cells", i, rows[i], column, width)
				}
			}
			if !strings.HasPrefix(rows[0], " Mindset ") || strings.Contains(rows[1], "Mindset") {
				t.Fatalf("title position: got %q, want title beside first row only", rows)
			}
		})
	}
}

func TestModel_SelectionWraps(t *testing.T) {
	m := New(t.Context(), "u", &reader{items: []data.ThoughtSummaryView{
		{ThoughtID: 1, Preview: "first"},
		{ThoughtID: 2, Preview: "second"},
		{ThoughtID: 3, Preview: "third"},
	}}, logging.Nop())
	m, _ = m.Update(m.Load(time.Now())())
	m.Resize(40, 2)
	m, _ = m.SetFocused(true)
	for _, step := range []struct {
		key  rune
		want int64
	}{
		{key: tea.KeyUp, want: 3},
		{key: tea.KeyDown, want: 1},
		{key: tea.KeyEnd, want: 3},
		{key: tea.KeyDown, want: 1},
		{key: tea.KeyPgUp, want: 2},
		{key: tea.KeyPgDown, want: 1},
	} {
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: step.key}))
		_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
		if got := cmd().(OpenThoughtMsg).Thought.ThoughtID; got != step.want {
			t.Fatalf("selection after key %d: got thought %d, want %d", step.key, got, step.want)
		}
		if m.index < m.offset || m.index >= m.offset+m.rows {
			t.Fatalf("selected row %d outside visible rows starting at %d", m.index, m.offset)
		}
	}
}
