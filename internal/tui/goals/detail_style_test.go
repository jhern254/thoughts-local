package goals

import (
	"charm.land/bubbles/v2/list"
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
				rawRows := strings.Split(m.View(), "\n")
				if got, want := rawRows[m.body.Height()+1], list.DefaultStyles(true).Title.Render("Events"); got != want {
					t.Fatalf("Events header block: got %q, want %q at the history heading", got, want)
				}
				if len(rows) > size[1] {
					t.Fatalf("screen height: got %d, want at most %d", len(rows), size[1])
				}
			})
		}
	}
}

func TestModel_DetailContentFocus(t *testing.T) {
	t.Run("focus moves between Goal settings and individual Subjects without selecting headers", func(t *testing.T) {
		m, service := activityModel(t)
		start, end := "2026-11-01", "2026-11-30"
		m.selected.StartDate = &start
		m.selected.EndDate = &end
		m.live = []data.Subject{{SubjectID: 1, SubjectName: "Programming"}, {SubjectID: 2, SubjectName: "Research"}}
		m.goalNumbers = map[int64]int64{1: 3, 2: 7}
		m.Resize(100, 35)
		activeStyle := goalListDelegate(true).Styles.SelectedTitle
		settings := goalSummary(*m.selected)
		contents := []string{settings, "Programming 3", "Research 7"}
		assertSelected := func(want int) {
			t.Helper()
			content := m.detailContent()
			for index, text := range contents {
				if got := strings.Contains(content, activeStyle.Render(text)); got != (index == want) {
					t.Fatalf("highlight for %q: got %v, want %v", text, got, index == want)
				}
			}
			header := m.list.Styles.Title.Render("Goals") + "\n" + service.item.GoalName + "\n"
			if !strings.HasPrefix(content, header) {
				t.Fatalf("Goal header: got %q, want prefix %q", content, header)
			}
			if !strings.Contains(content, m.list.Styles.Title.Render("Subjects")+"\n") {
				t.Fatal("Subjects heading lost its block style")
			}
			for _, row := range strings.Split(content, "\n") {
				if strings.Contains(ansi.Strip(row), "Created ") || strings.Contains(ansi.Strip(row), "Updated ") {
					if strings.Contains(row, "\x1b") {
						t.Fatalf("record timestamp was highlighted: %q", row)
					}
				}
			}
		}
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		assertSelected(0)
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		assertSelected(1)
		m, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		assertSelected(2)
		m, _ = m.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
		assertSelected(1)
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		assertSelected(-1)
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		assertSelected(1)
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
		assertSelected(0)
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
		assertSelected(2)
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

func TestModel_DetailFocusScrolling(t *testing.T) {
	t.Run("selection scrolls each Subject into view and clicks select the visible line", func(t *testing.T) {
		m, _ := activityModel(t)
		m.goalNumbers = make(map[int64]int64)
		for i := range 20 {
			id := int64(i + 1)
			m.live = append(m.live, data.Subject{SubjectID: id, SubjectName: fmt.Sprintf("Subject %02d", i+1)})
			m.goalNumbers[id] = 1
		}
		m.Resize(48, 18)
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
		for i := range 20 {
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
			want := fmt.Sprintf("Subject %02d", i+1)
			if !strings.Contains(ansi.Strip(m.body.View()), want) {
				t.Fatalf("focused Subject not scrolled into view: want %q, got %q", want, m.body.View())
			}
		}
		m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
		for range 4 {
			m, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown, Y: 1})
		}
		content := m.detailContent()
		selected := goalListDelegate(true).Styles.SelectedTitle.Render("Subject 04 1")
		if !strings.Contains(content, selected) {
			t.Fatal("wheel did not select one Subject at a time")
		}
		clickedRow := -1
		for index, row := range strings.Split(ansi.Strip(m.body.View()), "\n") {
			if strings.Contains(row, "Subject 03 1") {
				clickedRow = index
				break
			}
		}
		if clickedRow < 0 {
			t.Fatal("expected Subject 03 to be visible before clicking")
		}
		m, _ = m.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 4, Y: clickedRow})
		if !strings.Contains(m.detailContent(), goalListDelegate(true).Styles.SelectedTitle.Render("Subject 03 1")) {
			t.Fatal("click did not select the visible Subject")
		}
		if strings.Contains(m.detailContent(), selected) {
			t.Fatal("click retained the previous bright selection")
		}

	})
}
