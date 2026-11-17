package goals

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/tui/appearance"
)

func TestModel_SectionDividerFocus(t *testing.T) {
	for _, width := range []int{100, 48} {
		for _, weight := range []data.BorderWeight{data.BorderNormal, data.BorderHeavy} {
			t.Run(fmt.Sprintf("Goals list at %d columns with %s borders", width, weight), func(t *testing.T) {
				m, _ := testModel(t)
				m.open = true
				m.Resize(width, 30)
				m.SetBorderWeight(weight)
				line := strings.Repeat(appearance.Border(weight).Top, width)
				focused := lipgloss.NewStyle().Foreground(m.list.Styles.Title.GetBackground()).Render(line)
				for _, focus := range []listControl{goalListControl, statusControl, subjectControl, priorityControl} {
					m.listFocus = focus
					var divider string
					for _, row := range strings.Split(m.View(), "\n") {
						if ansi.Strip(row) == line {
							divider = row
							break
						}
					}
					want := line
					if focus == goalListControl || focus == priorityControl {
						want = focused
					}
					if divider != want {
						t.Fatalf("divider for focus %d: got %q, want %q", focus, divider, want)
					}
				}
			})
			t.Run(fmt.Sprintf("Details at %d columns with %s borders", width, weight), func(t *testing.T) {
				m, _ := activityModel(t)
				m.Resize(width, 30)
				m.SetBorderWeight(weight)
				line := strings.Repeat(appearance.Border(weight).Top, width)
				focused := lipgloss.NewStyle().Foreground(m.list.Styles.Title.GetBackground()).Render(line)
				for _, want := range []string{focused, line, focused} {
					rows := strings.Split(m.View(), "\n")
					if got := rows[m.body.Height()]; got != want {
						t.Fatalf("Details divider: got %q, want %q", got, want)
					}
					if got := ansi.StringWidth(rows[m.body.Height()]); got != width {
						t.Fatalf("divider width: got %d, want %d", got, width)
					}
					m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
				}
			})
		}
	}
}
