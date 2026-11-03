package goals

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestModel_FormPresentation(t *testing.T) {
	t.Run("creation starts on today's local date with examples and spaced inputs", func(t *testing.T) {
		m, _ := testModel(t)
		m.now = func() time.Time { return time.Date(2026, 10, 29, 23, 30, 0, 0, time.FixedZone("local", -7*3600)) }
		m.startForm(true)
		if got := m.fields[3].Value(); got != "2026-10-29" {
			t.Fatalf("start date got %q, want local today", got)
		}
		if got := m.fields[4].Value(); got != "" {
			t.Fatalf("end date got %q, want empty", got)
		}
		rows := strings.Split(ansi.Strip(m.formContent()), "\n")
		labels := []string{"Hours", "Minutes", "Start date", "End date", "Cadence:", "Priority:", "Status:", "Subjects:", "Save"}
		for _, label := range labels {
			found := false
			for i, row := range rows {
				if strings.Contains(row, label) {
					found = true
					if i == 0 || strings.TrimSpace(rows[i-1]) != "" {
						t.Errorf("%s must have a blank row above it; rows=%q", label, rows)
					}
				}
			}
			if !found {
				t.Errorf("missing field %s", label)
			}
		}
		for _, label := range []string{"Start date", "End date"} {
			for _, row := range rows {
				if strings.Contains(row, label) && !strings.Contains(row, "e.g. 2026-10-29") {
					t.Errorf("missing date example: %q", row)
				}
			}
		}
	})
	t.Run("editing retains existing dates including no start date", func(t *testing.T) {
		m, s := testModel(t)
		m.selected = &s.item
		m.startForm(false)
		if m.fields[3].Value() != "" {
			t.Fatal("editing an undated Goal supplied today's date")
		}
		start, end := "2025-02-03", "2027-03-04"
		s.item.StartDate = &start
		s.item.EndDate = &end
		m.startForm(false)
		if m.fields[3].Value() != start || m.fields[4].Value() != end {
			t.Fatal("editing replaced the persisted date bounds")
		}
	})
	t.Run("every focused input stays visible after spacing in a narrow window", func(t *testing.T) {
		m, _ := testModel(t)
		m.open = true
		m.Resize(48, 16)
		m.startForm(true)
		for i := range formCreateSave + 1 {
			m.refreshBody()
			view := ansi.Strip(m.View())
			if i < len(m.fields) {
				value := []string{"Name sample", "2", "30", "2026-10-29", "2027-01-01"}[i]
				m.fields[i].SetValue(value)
				m.refreshBody()
				view = ansi.Strip(m.View())
				if !strings.Contains(view, value) {
					t.Errorf("focused input %d not visible: %s", i, view)
				}
			} else {
				label := []string{"Cadence:", "Priority:", "Status:", "Subjects:", "Save"}[i-len(m.fields)]
				if !strings.Contains(view, label) {
					t.Errorf("focused action %s not visible: %s", label, view)
				}
			}
			m, _ = press(m, "down")
		}
	})
}

func TestModel_FormFocusHighlight(t *testing.T) {
	for _, create := range []bool{true, false} {
		for _, tc := range []struct {
			focus int
			label string
		}{{formName, "Name"}, {formHours, "Hours"}, {formMinutes, "Minutes"}, {formStartDate, "Start date"}, {formEndDate, "End date"}, {formCadence, "Cadence:"}, {formPriority, "Priority:"}, {formStatus, "Status:"}, {formSubjects, "Subjects:"}, {formCreateSave, "Save"}} {
			if !create && tc.focus == formSubjects {
				tc.label = "Save"
			}
			if !create && tc.focus == formCreateSave {
				continue
			}
			t.Run(fmt.Sprintf("create=%t/%s", create, tc.label), func(t *testing.T) {
				m, s := testModel(t)
				m.selected = &s.item
				m.startForm(create)
				m.focus = tc.focus
				m.list.Styles.Title = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230"))
				rendered := m.formContent()
				highlighted := m.list.Styles.Title.Background(lipgloss.Color("#FFB000")).Foreground(lipgloss.Color("#1A1A1A")).Render("› " + tc.label)
				if !strings.Contains(rendered, highlighted) {
					t.Fatalf("focused %s lacks the standard highlight: %q", tc.label, rendered)
				}
				if tc.focus == formStartDate || tc.focus == formEndDate {
					if !strings.Contains(rendered, highlighted+" (optional; e.g. 2026-10-29)") {
						t.Fatalf("date guidance must follow the highlight as plain text: %q", rendered)
					}
				}
			})
		}
	}
}

func TestModel_FormTitleAndNavigation(t *testing.T) {
	for _, create := range []bool{true, false} {
		t.Run(fmt.Sprintf("create=%t", create), func(t *testing.T) {
			m, s := testModel(t)
			m.open = true
			m.selected = &s.item
			m.startForm(create)
			m.list.Styles.Title = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230"))
			title := "Create Goal"
			if !create {
				title = "Edit Goal"
			}
			if !strings.HasPrefix(m.formContent(), m.list.Styles.Title.Render("Goal")+"\n"+title+"\n\n") {
				t.Fatal("form lacks colored Goal heading above plain form title")
			}
			m, _ = press(m, "tab")
			if m.focus != formName {
				t.Fatal("Tab moved between form fields")
			}
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
			if m.focus != m.formSaveFocus() {
				t.Fatal("Up did not wrap to Save")
			}
			m, _ = press(m, "down")
			if m.focus != formName {
				t.Fatal("Down did not wrap to Name")
			}
		})
	}
}
