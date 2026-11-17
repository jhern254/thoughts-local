package goals

import (
	tea "charm.land/bubbletea/v2"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/tui/appearance"
)

func TestModel_DetailSections(t *testing.T) {
	for _, size := range [][2]int{{100, 35}, {48, 18}} {
		for _, weight := range []data.BorderWeight{data.BorderNormal, data.BorderHeavy} {
			t.Run(fmt.Sprintf("divider fits %dx%d with %s borders", size[0], size[1], weight), func(t *testing.T) {
				m, _ := activityModel(t)
				m.Resize(size[0], size[1])
				m.SetBorderWeight(weight)
				rows := strings.Split(ansi.Strip(m.View()), "\n")
				divider := strings.Repeat(appearance.Border(weight).Top, size[0])
				if got := rows[m.body.Height()]; got != divider {
					t.Fatalf("details divider: got %q, want %q", got, divider)
				}
				if got := rows[m.body.Height()+1]; got != "ACTIVITY HISTORY" {
					t.Fatalf("history heading: got %q, want unchanged position", got)
				}
				if len(rows) > size[1] {
					t.Fatalf("screen height: got %d, want at most %d", len(rows), size[1])
				}
			})
		}
	}
}

func TestModel_DetailContentFocus(t *testing.T) {
	t.Run("focus highlights the Goal settings and Subject names but leaves headers and timestamps unchanged", func(t *testing.T) {
		m, service := activityModel(t)
		start, end := "2026-11-01", "2026-11-30"
		m.selected.StartDate = &start
		m.selected.EndDate = &end
		m.live = []data.Subject{{SubjectID: 1, SubjectName: "Programming"}, {SubjectID: 2, SubjectName: "Research"}}
		m.goalNumbers = map[int64]int64{1: 3, 2: 7}
		m.Resize(100, 35)
		before := m.detailContent()
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		focused := m.detailContent()
		activeStyle := goalListDelegate(true).Styles.SelectedTitle
		inactiveStyle := goalListDelegate(false).Styles.SelectedTitle
		for _, content := range []string{goalSummary(*m.selected), "Programming 3\nResearch 7"} {
			if !strings.Contains(focused, activeStyle.Render(content)) {
				t.Fatalf("focused details missing grouped highlight for %q: %q", content, focused)
			}
			if !strings.Contains(before, inactiveStyle.Render(content)) {
				t.Fatalf("unfocused details missing muted content for %q: %q", content, before)
			}
		}
		heading := m.list.Styles.Title.Render("Subjects")
		for _, content := range []string{before, focused} {
			if !strings.HasPrefix(content, service.item.GoalName+"\n\n") {
				t.Fatalf("Goal heading changed with focus: %q", content)
			}
			if !strings.Contains(content, heading+"\n") {
				t.Fatalf("Subjects heading missing shared block style: %q", content)
			}
			rows := strings.Split(content, "\n")
			for _, row := range rows {
				if strings.Contains(ansi.Strip(row), "Created ") || strings.Contains(ansi.Strip(row), "Updated ") {
					if strings.Contains(row, "\x1b") {
						t.Fatalf("record timestamp was highlighted: %q", row)
					}
				}
			}
		}
		if ansi.Strip(before) != ansi.Strip(focused) {
			t.Fatal("focus changed the detail text or geometry")
		}
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		if got := m.detailContent(); got != before {
			t.Fatal("return to history did not restore muted details")
		}
	})
	t.Run("long Unicode Subject names wrap inside the selection rail", func(t *testing.T) {
		m, _ := activityModel(t)
		m.live = []data.Subject{{SubjectID: 1, SubjectName: strings.Repeat("日本語の学習", 8)}}
		m.Resize(48, 18)
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		for _, row := range strings.Split(m.detailContent(), "\n") {
			if width := ansi.StringWidth(row); width > 48 {
				t.Fatalf("detail row width: got %d, want at most 48", width)
			}
		}
		for range 80 {
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		}
		if !strings.Contains(ansi.Strip(m.View()), "Updated ") {
			t.Fatal("wrapping hid the metadata footer from scrolling")
		}
	})
}
