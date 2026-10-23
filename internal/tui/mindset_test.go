package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
)

func newMindsetHome(t *testing.T, count int) Model {
	t.Helper()
	texts := make([]string, count)
	for i := range count {
		texts[i] = fmt.Sprintf("Practice %d deliberately 界", i+1)
	}
	return newMindsetHomeTexts(t, texts...)
}

func newMindsetHomeTexts(t *testing.T, texts ...string) Model {
	t.Helper()
	service := thought.NewService(testutils.NewFakeThoughtStore())
	for i, text := range texts {
		item, err := service.Create(t.Context(), "u", text, nil, time.Unix(int64(i), 0))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.SetMindset(t.Context(), "u", item.ThoughtID, true, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	m := NewModel(t.Context(), &data.User{UserID: "u"}, &subjectServiceStub{}, service, &metricsStub{}, &homeEventStub{}, &homeTimelineStub{}, logging.Nop())
	// Focus the panel before delivery; timer behavior is tested with explicit ticks
	// in the panel package, while this test executes real data command batches.
	m.focusHome(homePanelMindset)
	m, cmd := rootUpdate(m, m.Init()())
	m = startHomeData(t, m, cmd)
	m.focusHome(homePanelEvents)
	return m
}

func TestModel_MindsetHome(t *testing.T) {
	t.Run("highlights only the divider above the focused Events or entity section", func(t *testing.T) {
		for _, width := range []int{80, 40} {
			m := newMindsetHome(t, 0)
			{
				resized, resizeCmd := rootUpdate(m, tea.WindowSizeMsg{Width: width, Height: 30})
				m = runHomeData(t, resized, resizeCmd)
			}
			for _, focus := range []homePanel{homePanelEvents, homePanelEntities, homePanelMindset, homePanelEvents} {
				dividers := []string{}
				for _, row := range strings.Split(m.View().Content, "\n") {
					if strings.ReplaceAll(ansi.Strip(row), "┬", "─") == strings.Repeat("─", width) {
						dividers = append(dividers, row)
					}
				}
				if len(dividers) != 2 {
					t.Fatalf("width %d: got %d dividers, want 2", width, len(dividers))
				}
				for i, want := range []bool{focus == homePanelEvents || focus == homePanelDays, focus == homePanelEntities} {
					if got := strings.Contains(dividers[i], "\x1b[38;5;62m"); got != want {
						t.Fatalf("focus %d, divider %d at width %d: highlighted=%v, want %v: %q", focus, i, width, got, want, dividers[i])
					}
				}
				m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
			}
		}
	})
	t.Run("separates the inline mindset header from Events", func(t *testing.T) {
		m := newMindsetHome(t, 0)
		{
			resized, resizeCmd := rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 32})
			m = runHomeData(t, resized, resizeCmd)
		}
		rows := strings.Split(ansi.Strip(m.View().Content), "\n")
		if got, want := rows[2], " Mindset  ..."; got != want {
			t.Fatalf("mindset row: got %q, want %q", got, want)
		}
		if rows[3] != "" {
			t.Fatalf("row after Mindset: got %q, want blank", rows[3])
		}
		if rows[4] != "" {
			t.Fatalf("row above graph: got %q, want blank", rows[4])
		}
		if got, want := strings.ReplaceAll(rows[11], "┬", "─"), strings.Repeat("─", 80); got != want {
			t.Fatalf("panel separator: got %q, want %q", got, want)
		}
		if !strings.HasPrefix(rows[12], " Events ") {
			t.Fatalf("row below separator: got %q, want Events header", rows[12])
		}
	})
	t.Run("expansion makes room in the graph and returning from detail restores selection", func(t *testing.T) {
		m := newMindsetHome(t, 6)
		{
			resized, resizeCmd := rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 32})
			m = runHomeData(t, resized, resizeCmd)
		}
		before := ansi.Strip(m.View().Content)
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
		expanded := ansi.Strip(m.View().Content)
		row := func(text, label string) int {
			for i, line := range strings.Split(text, "\n") {
				if strings.Contains(line, label) {
					return i
				}
			}
			return -1
		}
		if got, want := row(expanded, "Events")-row(before, "Events"), 0; got != want {
			t.Fatalf("Events displacement: got %d, want %d", got, want)
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnd}))
		selected := m.View().Content
		m, cmd := rootUpdate(m, enterKey())
		m = runHomeData(t, m, cmd)
		if m.screen != screenMindsetThought || !strings.Contains(m.View().Content, "Practice 6") {
			t.Fatalf("detail: got screen %v, %q", m.screen, m.View().Content)
		}
		m, _ = rootUpdate(m, escapeKey())
		if m.screen != screenEvents || m.View().Content != selected {
			t.Fatal("return lost panel selection, scroll, or timeline")
		}
		m, cmd = rootUpdate(m, runeKey('q'))
		if cmd != nil || m.View().Content != selected || m.homeFocus() != homePanelMindset {
			t.Fatal("Q must leave the focused picker and cursor unchanged")
		}
		m, _ = rootUpdate(m, escapeKey())
		if !m.exitPromptOpen || m.homeFocus() != homePanelMindset || !m.mindsets.Expanded() || !strings.Contains(m.View().Content, "Exit app?") {
			t.Fatal("Escape must open the app exit prompt without leaving Mindset")
		}
		m, _ = rootUpdate(m, escapeKey())
		if m.exitPromptOpen || m.View().Content != selected {
			t.Fatal("canceling exit must preserve picker focus, cursor, and scroll")
		}
	})
	t.Run("Left preserves the focused picker and its cursor", func(t *testing.T) {
		for _, count := range []int{0, 3} {
			m := newMindsetHome(t, count)
			m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
			m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnd}))
			day := m.events.Day()
			before := m.View().Content
			for range 2 {
				var cmd tea.Cmd
				m, cmd = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft}))
				if cmd != nil {
					t.Fatal("Left returned a command, want no action")
				}
				if m.homeFocus() != homePanelMindset || !m.mindsets.Expanded() || m.View().Content != before || !m.events.Day().Equal(day) {
					t.Fatalf("Left with %d thoughts: got focus %d expanded=%v day=%v, want unchanged expanded Mindset on %v", count, m.homeFocus(), m.mindsets.Expanded(), m.events.Day(), day)
				}
			}
		}
	})
	t.Run("Down from the last thought wraps to the first and opens it", func(t *testing.T) {
		m := newMindsetHome(t, 6)
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyEnd}))
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
		selected := m.View().Content
		m, cmd := rootUpdate(m, enterKey())
		m = runHomeData(t, m, cmd)
		if m.screen != screenMindsetThought || !strings.Contains(m.View().Content, "Practice 1 deliberately") {
			t.Fatalf("wrapped detail: got %q, want Practice 1", m.View().Content)
		}
		m, _ = rootUpdate(m, escapeKey())
		if m.View().Content != selected {
			t.Fatal("detail return changed wrapped selection or scroll")
		}
	})
	t.Run("Tab cycles the three panels and reverse Tab returns", func(t *testing.T) {
		m := newMindsetHome(t, 0)
		for _, want := range []homePanel{homePanelEntities, homePanelMindset, homePanelEvents} {
			m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
			if got := m.homeFocus(); got != want {
				t.Fatalf("focus: got %d, want %d", got, want)
			}
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
		if m.homeFocus() != homePanelMindset {
			t.Fatal("reverse Tab did not focus Mindset")
		}
	})
	t.Run("narrow expansion preserves usable Events height and clips Unicode rows", func(t *testing.T) {
		m := newMindsetHome(t, 6)
		{
			resized, resizeCmd := rootUpdate(m, tea.WindowSizeMsg{Width: 40, Height: 24})
			m = runHomeData(t, resized, resizeCmd)
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
		if got, want := m.homeEventsHeight, 14; got != want {
			t.Fatalf("Events height: got %d, want %d", got, want)
		}
		if strings.Contains(m.events.View(), "Resize terminal") {
			t.Fatal("expanded panel made a usable timeline unusable")
		}
		for _, line := range strings.Split(m.View().Content, "\n") {
			if got := ansi.StringWidth(line); got > 40 {
				t.Fatalf("line width: got %d, want <=40: %q", got, line)
			}
		}
	})
	t.Run("m from panel detail updates status without leaving the thought", func(t *testing.T) {
		m := newMindsetHome(t, 1)
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
		m, cmd := rootUpdate(m, enterKey())
		m = runHomeData(t, m, cmd)
		m, cmd = rootUpdate(m, runeKey('m'))
		blocked, _ := rootUpdate(m, escapeKey())
		if blocked.screen != screenMindsetThought {
			t.Fatal("pending designation allowed back navigation")
		}
		m = runHomeData(t, m, cmd)
		if m.screen != screenMindsetThought || strings.HasPrefix(ansi.Strip(m.thoughts.View()), " M ") {
			t.Fatalf("unmarked detail: got %q", m.thoughts.View())
		}
	})
}

func TestModel_MindsetReadingLayout(t *testing.T) {
	t.Run("wrapped thought grows above the separated timeline and reserves the right margin", func(t *testing.T) {
		text := "Practice deliberately and give the whole sentence enough room to be read without cutting off its ending.\n\nKeep this paragraph too."
		for _, size := range [][2]int{{80, 40}, {40, 30}, {24, 24}} {
			t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
				m := newMindsetHomeTexts(t, text)
				{
					resized, cmd := rootUpdate(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
					m = runHomeData(t, resized, cmd)
				}
				m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
				view := ansi.Strip(m.View().Content)
				rows := strings.Split(view, "\n")
				if len(rows) > size[1] {
					t.Fatalf("screen height: got %d, want <=%d", len(rows), size[1])
				}
				separator := -1
				for i, row := range rows {
					if strings.ReplaceAll(row, "┬", "─") == strings.Repeat("─", size[0]) {
						separator = i
						break
					}
				}
				graphRows := 2
				if size[1] == 40 {
					graphRows = 11
				}
				if size[1] == 24 {
					graphRows = 1
				}
				if separator < graphRows+1 || rows[separator-graphRows-1] != "" || !strings.HasPrefix(rows[separator+1], " Events ") {
					t.Fatalf("Mindset separation: got %q", view)
				}
				for i := 2; i < separator-graphRows-1; i++ {
					if got := ansi.StringWidth(rows[i]); got > size[0]-1 {
						t.Fatalf("Mindset row %d width: got %d, want <=%d", i, got, size[0]-1)
					}
				}
				if size[0] == 80 && !strings.Contains(view, "Keep this paragraph too.") {
					t.Fatalf("complete paragraphs: got %q", view)
				}
				if !strings.Contains(view, "Subjects") || strings.Contains(view, "Resize terminal") {
					t.Fatalf("home controls unavailable: %q", view)
				}
			})
		}
	})
	t.Run("reading keys keep the tall thought selected and detail return restores the scroll", func(t *testing.T) {
		lines := []string{}
		for i := range 20 {
			lines = append(lines, fmt.Sprintf("Reading line %02d", i+1))
		}
		text := strings.Join(lines, "\n")
		m := newMindsetHomeTexts(t, text, "Second thought")
		{
			resized, cmd := rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 34})
			m = runHomeData(t, resized, cmd)
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}))
		for range 3 {
			m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
		}
		selected := m.View().Content
		if !strings.Contains(ansi.Strip(selected), "Reading line 04") || strings.Contains(ansi.Strip(selected), "Reading line 01") {
			t.Fatalf("reading position: got %q", selected)
		}
		m, cmd := rootUpdate(m, enterKey())
		m = runHomeData(t, m, cmd)
		if m.screen != screenMindsetThought || !strings.Contains(ansi.Strip(m.View().Content), "Reading line 01") {
			t.Fatalf("opened thought: got screen %d, %q", m.screen, m.View().Content)
		}
		m, _ = rootUpdate(m, escapeKey())
		if m.View().Content != selected || m.homeFocus() != homePanelMindset {
			t.Fatalf("detail return: got %q, want preserved reading position", m.View().Content)
		}
		{
			resized, cmd := rootUpdate(m, tea.WindowSizeMsg{Width: 40, Height: 30})
			m = runHomeData(t, resized, cmd)
		}
		if !strings.Contains(ansi.Strip(m.View().Content), "Reading line 04") {
			t.Fatalf("resize lost reading position: %q", m.View().Content)
		}
	})
}
