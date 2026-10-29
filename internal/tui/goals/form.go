package goals

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/goal"
	"github.com/jhern254/go-thoughts/internal/logging"
)

const cadenceCount = 6

func cadenceAt(index int) string {
	return [...]string{"daily", "weekly", "monthly", "mtd", "quarterly", "yearly"}[index]
}
func cadenceLabel(value string) string {
	switch value {
	case "daily":
		return "day"
	case "weekly":
		return "week"
	case "monthly":
		return "month"
	case "mtd":
		return "month to date"
	case "quarterly":
		return "quarter"
	case "yearly":
		return "year"
	}
	return value
}

func targetSeconds(hours, minutes string) (int64, error) {
	var total int64
	for i, value := range []string{hours, minutes} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		number, err := strconv.ParseInt(value, 10, 64)
		multiplier := int64(3600)
		if i == 1 {
			multiplier = 60
		}
		if err != nil || number < 0 || number > (math.MaxInt64-total)/multiplier {
			return 0, errors.New("Enter nonnegative hours and minutes within the supported range.")
		}
		total += number * multiplier
	}
	if total == 0 {
		return 0, errors.New("Enter a target greater than zero.")
	}
	return total, nil
}
func formatDuration(seconds int64) string {
	hours := seconds / 3600
	minutes := (seconds % 3600) / 60
	value := fmt.Sprintf("%dh %dm", hours, minutes)
	if seconds%60 != 0 {
		value += fmt.Sprintf(" %ds", seconds%60)
	}
	return value
}
func (m *Model) startForm(create bool) {
	m.status = ""
	for i := range m.fields {
		m.fields[i].Blur()
	}
	m.focus = 0
	if create {
		m.screen = createView
		m.selected = nil
		m.selectedIDs = make(map[int64]bool)
		m.cadence = 1
		for i := range m.fields {
			m.fields[i].SetValue("")
		}
		m.fields[3].SetValue(m.now().Format(time.DateOnly))
	} else {
		m.screen = editView
		m.populateForm()
	}
	m.body.GotoTop()
}
func (m *Model) populateForm() {
	item := m.selected
	m.fields[0].SetValue(item.GoalName)
	m.fields[1].SetValue(strconv.FormatInt(item.TargetSeconds/3600, 10))
	m.fields[2].SetValue(strconv.FormatInt(item.TargetSeconds%3600/60, 10))
	m.fields[3].SetValue("")
	m.fields[4].SetValue("")
	if item.StartDate != nil {
		m.fields[3].SetValue(*item.StartDate)
	}
	if item.EndDate != nil {
		m.fields[4].SetValue(*item.EndDate)
	}
	for i := range cadenceCount {
		value := cadenceAt(i)
		if value == item.Cadence {
			m.cadence = i
		}
	}
}
func (m *Model) updateForm(msg tea.Msg) tea.Cmd {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "tab", "shift+tab":
			step := 1
			if key.String() == "shift+tab" {
				step = -1
			}
			count := 7
			if m.screen == createView {
				count = 8
			}
			m.focus = (m.focus + step + count) % count
			for i := range m.fields {
				m.fields[i].Blur()
			}
			if m.focus < 5 {
				return m.fields[m.focus].Focus()
			}
			return nil
		case "left", "right":
			if m.focus == 5 {
				step := 1
				if key.String() == "left" {
					step = -1
				}
				m.cadence = (m.cadence + step + cadenceCount) % cadenceCount
				return nil
			}
		case "enter":
			if m.focus == 5 {
				m.cadence = (m.cadence + 1) % cadenceCount
				return nil
			}
			if m.screen == createView && m.focus == 6 {
				m.pickerBefore = cloneSelection(m.selectedIDs)
				m.pickerReturn = createView
				m.screen = subjectsView
				return m.loadPicker()
			}
			return m.saveProperties()
		case "ctrl+s":
			return m.saveProperties()
		}
	}
	if m.focus < 5 {
		var cmd tea.Cmd
		m.fields[m.focus], cmd = m.fields[m.focus].Update(msg)
		return cmd
	}
	return nil
}
func (m Model) formContent() string {
	title := "Create Goal"
	if m.screen == editView {
		title = "Edit Goal"
	}
	lines := []string{title, ""}
	labels := []string{"Name", "Hours", "Minutes", "Start date", "End date"}
	for i, label := range labels {
		heading := m.formHeading(i, label)
		if i == 3 || i == 4 {
			heading += " (optional; e.g. 2026-10-29)"
		}
		lines = append(lines, heading, m.fields[i].View(), "")
	}
	lines = append(lines, m.formHeading(5, "Cadence:")+" "+cadenceLabel(cadenceAt(m.cadence)), "")
	saveFocus := 6
	if m.screen == createView {
		lines = append(lines, m.formHeading(6, "Subjects:")+fmt.Sprintf(" %d selected (Enter to choose)", len(m.ids())), "")
		saveFocus = 7
	}
	lines = append(lines, m.formHeading(saveFocus, "Save"))

	if m.screen == editView && m.selected.TargetSeconds%60 != 0 {
		lines = append(lines, "Current target: "+formatDuration(m.selected.TargetSeconds), "Changing duration uses whole minutes.")
	}
	return strings.Join(lines, "\n")
}
func (m Model) formHeading(index int, label string) string {
	if m.focus == index {
		return m.list.Styles.Title.Render("› " + label)
	}
	return "  " + label
}

func (m Model) ids() []int64 {
	ids := make([]int64, 0)
	for id, selected := range m.selectedIDs {
		if selected {
			ids = append(ids, id)
		}
	}
	return ids
}
func (m *Model) saveProperties() tea.Cmd {
	create := m.screen == createView
	var target int64
	if !create && m.fields[1].Value() == strconv.FormatInt(m.selected.TargetSeconds/3600, 10) && m.fields[2].Value() == strconv.FormatInt(m.selected.TargetSeconds%3600/60, 10) {
		target = m.selected.TargetSeconds
	} else {
		var err error
		target, err = targetSeconds(m.fields[1].Value(), m.fields[2].Value())
		if err != nil {
			m.status = err.Error()
			return nil
		}
	}

	input := goal.CreateGoalInput{
		Name:          m.fields[0].Value(),
		TargetSeconds: target,
		Cadence:       cadenceAt(m.cadence),
		StartDate:     m.fields[3].Value(),
		EndDate:       m.fields[4].Value(),
	}
	operation := logging.GoalUpdate
	if create {
		operation = logging.GoalCreate
	}
	ctx := m.beginRequest(operation)
	m.saving = true
	m.status = "Saving…"
	owner, user, assignments, service, ids := m.owner, m.userID, m.assignments, m.goals, m.ids()
	var item data.Goal
	if !create {
		item = *m.selected
	}
	return func() tea.Msg {
		var saved *data.Goal
		var err error
		if create {
			saved, err = assignments.Create(ctx, user, input, ids)
		} else {
			saved, err = service.Update(ctx, user, item.GoalID, item.Version, goal.UpdateGoalInput{
				Name:           input.Name,
				TargetSeconds:  target,
				Cadence:        input.Cadence,
				StartDate:      input.StartDate,
				EndDate:        input.EndDate,
				TZ:             item.TZ,
				WeekStart:      item.WeekStart,
				DefaultCadence: optionalValue(item.DefaultCadence),
			})
		}
		return result{owner: owner, item: saved, err: err}
	}
}
func optionalValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
func (m *Model) saveSubjects() tea.Cmd {
	ctx := m.beginRequest(logging.GoalSubjectsReplace)
	m.saving = true
	m.status = "Saving…"
	owner, user, item, ids, service := m.owner, m.userID, *m.selected, m.ids(), m.assignments
	return func() tea.Msg {
		saved, err := service.ReplaceSubjects(ctx, user, item.GoalID, item.Version, ids)
		return result{owner: owner, item: saved, err: err}
	}
}
func errorMessage(err error) string {
	if errors.Is(err, data.ErrVersionConflict) {
		return "Goal changed. R: reload (discard edits) · Esc: cancel."
	}
	var validation *goal.ValidationError
	if errors.As(err, &validation) {
		for _, field := range []string{"goal_name", "target_seconds", "goal_start_date", "goal_end_date", "cadence"} {
			if message := validation.PublicFields()[field]; message != "" {
				return field + ": " + message
			}
		}
	}
	if errors.Is(err, data.ErrRecordNotFound) {
		return "Goal or Subject unavailable. R: reload · Esc: cancel."
	}
	if errors.Is(err, data.ErrDuplicateRecord) {
		return "A Goal with this name already exists."
	}
	return "Could not complete Goal operation. R: retry or reload · Esc: cancel."
}
