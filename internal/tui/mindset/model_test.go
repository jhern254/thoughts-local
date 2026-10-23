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
	items       []data.MindsetThought
	from, until time.Time
	err         error
}

func (s *reader) ListMindsetsInRange(_ context.Context, _ string, from, until time.Time) ([]data.MindsetThought, error) {
	s.from, s.until = from, until
	return s.items, s.err
}
func TestModel_Panel(t *testing.T) {
	t.Run("rotates in order and pauses while focused", func(t *testing.T) {
		service := &reader{items: []data.MindsetThought{{ThoughtID: 1, Text: "first"}, {ThoughtID: 2, Text: "second"}}}
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
		service := &reader{items: []data.MindsetThought{{ThoughtID: 1, Text: "one"}, {ThoughtID: 2, Text: "two"}, {ThoughtID: 3, Text: "three"}}}
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
		service := &reader{items: []data.MindsetThought{{ThoughtID: 1, Text: "old"}}}
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
		service.items = []data.MindsetThought{{ThoughtID: 1, Text: "recovered"}}
		m, _ = m.SetFocused(true)
		m, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: 'r', Text: "r"}))
		m, _ = m.Update(cmd())
		if !strings.Contains(m.View(), "recovered") {
			t.Fatalf("retry: got %q", m.View())
		}
	})
	t.Run("hiding cancels the timer and queued ticks cannot rotate", func(t *testing.T) {
		service := &reader{items: []data.MindsetThought{{ThoughtID: 1, Text: "one"}, {ThoughtID: 2, Text: "two"}}}
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
		service := &reader{items: []data.MindsetThought{{ThoughtID: 1, Text: "one"}, {ThoughtID: 2, Text: "two"}}}
		m := New(t.Context(), "u", service, logging.Nop())
		cmd := m.Load(time.Now())
		m, _ = m.Update(cmd())
		m.index = 1
		m, _ = m.SetFocused(true)
		service.items = []data.MindsetThought{{ThoughtID: 2, Text: "two"}, {ThoughtID: 1, Text: "one"}}
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
		items []data.MindsetThought
		want  string
	}{
		{name: "empty", want: " Mindset  ..."},
		{name: "populated", items: []data.MindsetThought{{ThoughtID: 1, Text: "Keep learning"}}, want: " Mindset  Keep learning"},
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
	t.Run("wraps Unicode within the remaining header width", func(t *testing.T) {
		m := New(t.Context(), "u", &reader{items: []data.MindsetThought{{ThoughtID: 1, Text: "界界界界界界界界"}}}, logging.Nop())
		m, _ = m.Update(m.Load(time.Now())())
		m.Resize(18, 12)
		rows := strings.Split(ansi.Strip(m.View()), "\n")
		if len(rows) != 3 || m.Height() != 3 {
			t.Fatalf("wrapped Unicode: got %q height=%d, want three rows", rows, m.Height())
		}
		for i, want := range []string{" Mindset  界界界", "          界界界", "          界界"} {
			if rows[i] != want {
				t.Fatalf("row %d: got %q, want %q", i, rows[i], want)
			}
		}
	})
}

func TestModel_RotationAfterFocus(t *testing.T) {
	m := New(t.Context(), "u", &reader{items: []data.MindsetThought{
		{ThoughtID: 1, Text: "first"},
		{ThoughtID: 2, Text: "second"},
		{ThoughtID: 3, Text: "third"},
	}}, logging.Nop())
	m, _ = m.Update(m.Load(time.Now())())
	m, _ = m.SetVisible(true)
	oldTick := tickMsg{owner: m.owner, generation: m.generation}
	m, _ = m.SetFocused(true)
	m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnd}))
	m, cmd := m.SetFocused(false)
	if cmd == nil || ansi.Strip(m.View()) != " Mindset  third" {
		t.Fatalf("blurred picker: got %q timer=%v, want selected thought and a fresh timer", m.View(), cmd != nil)
	}
	m, _ = m.Update(oldTick)
	if got := ansi.Strip(m.View()); got != " Mindset  third" {
		t.Fatalf("stale tick changed selected preview: got %q, want third", got)
	}
	m, _ = m.Update(tickMsg{owner: m.owner, generation: m.generation})
	if got := ansi.Strip(m.View()); got != " Mindset  first" {
		t.Fatalf("next cycle: got %q, want first after third", got)
	}
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
	for _, width := range []int{80, 40, 24} {
		t.Run(fmt.Sprintf("width %d", width), func(t *testing.T) {
			m := New(t.Context(), "u", &reader{items: []data.MindsetThought{{ThoughtID: 1, Text: "First 界 thought"}, {ThoughtID: 2, Text: "Second 界 thought with a long sentence"}}}, logging.Nop())
			m, _ = m.Update(m.Load(time.Now())())
			m.Resize(width, 12)
			m, _ = m.SetFocused(true)
			rows := strings.Split(ansi.Strip(m.View()), "\n")
			if m.Height() != len(rows) {
				t.Fatalf("height: got %d, want %d", m.Height(), len(rows))
			}
			for i, row := range rows {
				if ansi.StringWidth(row) > width-1 {
					t.Fatalf("row %d width: got %d, want <=%d", i, ansi.StringWidth(row), width-1)
				}
				if i == 0 && !strings.HasPrefix(row, " Mindset │First") {
					t.Fatalf("first row: got %q, want heading and selected text", row)
				}
				if i > 0 && (!strings.HasPrefix(row, "         ") || strings.Contains(row, "Mindset")) {
					t.Fatalf("continuation: got %q, want text beside heading", row)
				}
			}
			if !strings.Contains(strings.Join(rows, ""), "sentence") {
				t.Fatalf("full list: got %q, want final word", rows)
			}
		})
	}
}

func TestModel_SelectionWraps(t *testing.T) {
	m := New(t.Context(), "u", &reader{items: []data.MindsetThought{
		{ThoughtID: 1, Text: "first"},
		{ThoughtID: 2, Text: "second"},
		{ThoughtID: 3, Text: "third"},
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

func TestModel_CompleteText(t *testing.T) {
	t.Run("wraps full text and preserves paragraphs", func(t *testing.T) {
		text := "Practice deliberately with a sentence that extends far beyond the former eighty character preview limit.\n\nSecond paragraph ends here."
		m := New(t.Context(), "u", &reader{items: []data.MindsetThought{{ThoughtID: 1, Text: text}}}, logging.Nop())
		m, _ = m.Update(m.Load(time.Now())())
		m.Resize(40, 12)
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "ends here.") || strings.Contains(view, "…") || m.Height() < 5 {
			t.Fatalf("complete text: got %q height=%d, want wrapped text through its final sentence", view, m.Height())
		}
	})
}

func newReadingPanel(t *testing.T, text string, rows int) Model {
	t.Helper()
	m := New(t.Context(), "u", &reader{items: []data.MindsetThought{{ThoughtID: 1, Text: text}, {ThoughtID: 2, Text: "Next thought"}}}, logging.Nop())
	m, _ = m.Update(m.Load(time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC))())
	m.Resize(40, rows)
	m, _ = m.SetFocused(true)
	return m
}

func TestModel_Reading(t *testing.T) {
	lines := []string{}
	for i := range 20 {
		lines = append(lines, fmt.Sprintf("Line %02d", i+1))
	}
	text := strings.Join(lines, "\n")
	press := func(m Model, key rune) Model { next, _ := m.Update(tea.KeyPressMsg(tea.Key{Code: key})); return next }
	t.Run("reads to the end before selecting the next thought and wraps both ways", func(t *testing.T) {
		m := newReadingPanel(t, text, 4)
		for i := 1; i <= 16; i++ {
			m = press(m, tea.KeyDown)
			if m.index != 0 || m.offset != i {
				t.Fatalf("reading step %d: got index=%d offset=%d, want 0,%d", i, m.index, m.offset, i)
			}
			if !strings.Contains(ansi.Strip(m.View()), fmt.Sprintf("Line %02d", i+4)) {
				t.Fatalf("step %d: missing last visible line in %q", i, m.View())
			}
		}
		m = press(m, tea.KeyDown)
		if m.index != 1 {
			t.Fatalf("next selection: got %d, want 1", m.index)
		}
		m = press(m, tea.KeyUp)
		if m.index != 0 || !strings.Contains(ansi.Strip(m.View()), "Line 20") {
			t.Fatalf("previous thought: got %q, want its end", m.View())
		}
		for range 16 {
			m = press(m, tea.KeyUp)
		}
		m = press(m, tea.KeyUp)
		if m.index != 1 {
			t.Fatalf("reverse wrap: got %d, want 1", m.index)
		}
		m = press(m, tea.KeyDown)
		if m.index != 0 || m.offset != 0 {
			t.Fatalf("forward wrap: got index=%d offset=%d, want 0,0", m.index, m.offset)
		}
	})
	t.Run("page keys read a viewport and home and end select boundaries", func(t *testing.T) {
		m := newReadingPanel(t, text, 4)
		m = press(m, tea.KeyPgDown)
		if m.index != 0 || m.offset != 4 {
			t.Fatalf("page down: got index=%d offset=%d, want 0,4", m.index, m.offset)
		}
		m = press(m, tea.KeyPgUp)
		if m.index != 0 || m.offset != 0 {
			t.Fatalf("page up: got index=%d offset=%d, want 0,0", m.index, m.offset)
		}
		m = press(m, tea.KeyEnd)
		if m.index != 1 || !strings.Contains(ansi.Strip(m.View()), "Next thought") {
			t.Fatalf("end: got %q index=%d", m.View(), m.index)
		}
		m = press(m, tea.KeyHome)
		if m.index != 0 || m.offset != 0 {
			t.Fatalf("home: got index=%d offset=%d, want 0,0", m.index, m.offset)
		}
	})
	t.Run("focus and resize retain the reading position", func(t *testing.T) {
		m := newReadingPanel(t, text, 4)
		for range 8 {
			m = press(m, tea.KeyDown)
		}
		before := m.View()
		m, _ = m.SetFocused(false)
		if !strings.Contains(ansi.Strip(m.View()), "Line 01") {
			t.Fatal("unfocused thought must start at its beginning")
		}
		m, _ = m.SetFocused(true)
		if m.View() != before {
			t.Fatalf("refocused: got %q, want %q", m.View(), before)
		}
		m.Resize(32, 6)
		if !strings.Contains(ansi.Strip(m.View()), "Line 09") || m.index != 0 {
			t.Fatalf("resized position: got %q index=%d", m.View(), m.index)
		}
	})
	t.Run("refresh retains selected thought and reading position after reordering", func(t *testing.T) {
		m := newReadingPanel(t, text, 4)
		for range 8 {
			m = press(m, tea.KeyDown)
		}
		service := m.reader.(*reader)
		service.items = []data.MindsetThought{service.items[1], service.items[0]}
		m, _ = m.Update(m.Load(m.day)())
		if m.index != 1 || !strings.Contains(ansi.Strip(m.View()), "Line 09") {
			t.Fatalf("refreshed position: got %q index=%d", m.View(), m.index)
		}
	})
}

func TestModel_TextLayout(t *testing.T) {
	t.Run("preserves blank paragraphs and highlights every selected line", func(t *testing.T) {
		m := newReadingPanel(t, "First paragraph.\n\nSecond paragraph.", 12)
		rows := strings.Split(m.View(), "\n")
		want := []string{" Mindset │First paragraph.", "         │", "         │Second paragraph.", "          Next thought"}
		if len(rows) != len(want) {
			t.Fatalf("rows: got %q, want %q", rows, want)
		}
		for i := range rows {
			if got := ansi.Strip(rows[i]); got != want[i] {
				t.Fatalf("row %d: got %q, want %q", i, got, want[i])
			}
			if i < 3 && !strings.Contains(rows[i], "38;2;173;88;180") {
				t.Fatalf("row %d: missing purple selection rail: %q", i, rows[i])
			}
		}
	})
	t.Run("wraps long words without losing Unicode characters", func(t *testing.T) {
		text := strings.Repeat("界", 21)
		m := newReadingPanel(t, text, 12)
		m.Resize(24, 12)
		m, _ = m.SetFocused(false)
		view := ansi.Strip(m.View())
		if got := strings.Count(view, "界"); got != 21 {
			t.Fatalf("Unicode characters: got %d, want 21: %q", got, view)
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > 23 {
				t.Fatalf("line width: got %d, want <=23: %q", ansi.StringWidth(line), line)
			}
		}
	})
	t.Run("limits height to twelve physical lines", func(t *testing.T) {
		m := newReadingPanel(t, strings.Repeat("line\n", 30), 100)
		if m.Height() != 12 || len(strings.Split(m.View(), "\n")) != 12 {
			t.Fatalf("panel height: got %d, want 12", m.Height())
		}
	})
	t.Run("normalizes line endings and tabs without allowing terminal controls", func(t *testing.T) {
		m := newReadingPanel(t, "one\r\n\r\ntwo\tthree\x07", 12)
		got := ansi.Strip(m.View())
		if strings.ContainsAny(got, "\r\t\x07") || !strings.Contains(got, "two    three") || !strings.Contains(got, "         │\n") {
			t.Fatalf("safe multiline text: got %q", got)
		}
	})
}

func TestModel_ReadingReflowAndRotation(t *testing.T) {
	t.Run("resize preserves position within consecutive blank lines", func(t *testing.T) {
		m := newReadingPanel(t, "First\n\n\n\nLast\nTail", 2)
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
		before := m.View()
		m.Resize(32, 2)
		if m.View() != before {
			t.Fatalf("blank-line reading position: got %q, want %q", m.View(), before)
		}
	})

	t.Run("reflows a wrapped paragraph around the same source text", func(t *testing.T) {
		words := []string{}
		for i := range 80 {
			words = append(words, fmt.Sprintf("word%02d", i))
		}
		m := newReadingPanel(t, strings.Join(words, " "), 4)
		for range 5 {
			m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
		}
		before := strings.Fields(ansi.Strip(m.View()))
		// The title and rail precede the first visible word.
		anchor := strings.TrimPrefix(before[1], "│")
		m.Resize(28, 4)
		if !strings.Contains(ansi.Strip(m.View()), anchor) {
			t.Fatalf("reflow: got %q, want visible source word %q", m.View(), anchor)
		}
	})
	t.Run("rotation changes height with the complete selected thought", func(t *testing.T) {
		m := newReadingPanel(t, "first\n\nlast", 12)
		m, _ = m.SetFocused(false)
		m, _ = m.SetVisible(true)
		if m.Height() != 3 {
			t.Fatalf("multiline height: got %d, want 3", m.Height())
		}
		m, _ = m.Update(tickMsg{owner: m.owner, generation: m.generation})
		if m.Height() != 1 || !strings.Contains(ansi.Strip(m.View()), "Next thought") {
			t.Fatalf("rotated thought: got %q height=%d, want short thought", m.View(), m.Height())
		}
		m, _ = m.Update(tickMsg{owner: m.owner, generation: m.generation})
		if m.Height() != 3 || !strings.Contains(ansi.Strip(m.View()), "last") {
			t.Fatalf("rotated multiline thought: got %q height=%d", m.View(), m.Height())
		}
	})
	t.Run("loading and failure replace a tall thought without stale rows", func(t *testing.T) {
		m := newReadingPanel(t, strings.Repeat("line\n", 20), 12)
		service := m.reader.(*reader)
		service.err = errors.New("private error")
		cmd := m.Load(m.day)
		if m.Height() != 1 || strings.Contains(m.View(), "line") {
			t.Fatalf("loading: got %q height=%d", m.View(), m.Height())
		}
		m, _ = m.Update(cmd())
		if m.Height() != 1 || !strings.Contains(m.View(), "Could not load mindsets") || strings.Contains(m.View(), "private") {
			t.Fatalf("failure: got %q height=%d", m.View(), m.Height())
		}
	})
}
