package goals

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/goal"
	"github.com/jhern254/go-thoughts/internal/logging"
)

const cadenceCount = 6

// Text inputs precede selectors and actions in keyboard navigation order.
const (
	formName = iota
	formHours
	formMinutes
	formStartDate
	formEndDate
	formCadence
	formPriority
	formStatus
	formSubjects
	formCreateSave
)

func (m Model) formSaveFocus() int {
	if m.screen == createView {
		return formCreateSave
	}
	return formSubjects // Editing has no Subjects action; Save occupies its position.
}

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
	m.focus = formName
	if create {
		m.screen = createView
		m.selected = nil
		m.selectedIDs = make(map[int64]bool)
		m.cadence = 1
		m.priority = "normal"
		m.isActive = true
		for i := range m.fields {
			m.fields[i].SetValue("")
		}
		m.fields[formStartDate].SetValue(m.now().Format(time.DateOnly))
	} else {
		m.screen = editView
		m.populateForm()
	}
	m.body.GotoTop()
}
func (m *Model) populateForm() {
	item := m.selected
	m.priority = item.Priority
	m.isActive = item.IsActive
	m.fields[formName].SetValue(item.GoalName)
	m.fields[formHours].SetValue(strconv.FormatInt(item.TargetSeconds/3600, 10))
	m.fields[formMinutes].SetValue(strconv.FormatInt(item.TargetSeconds%3600/60, 10))
	m.fields[formStartDate].SetValue("")
	m.fields[formEndDate].SetValue("")
	if item.StartDate != nil {
		m.fields[formStartDate].SetValue(*item.StartDate)
	}
	if item.EndDate != nil {
		m.fields[formEndDate].SetValue(*item.EndDate)
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
		case "up", "down":
			step := 1
			if key.String() == "up" {
				step = -1
			}
			count := m.formSaveFocus() + 1
			m.focus = (m.focus + step + count) % count
			for i := range m.fields {
				m.fields[i].Blur()
			}
			if m.focus < len(m.fields) {
				return m.fields[m.focus].Focus()
			}
			return nil
		case "left", "right":
			if m.focus >= formCadence && m.focus <= formStatus {
				step := 1
				if key.String() == "left" {
					step = -1
				}
				m.changeFormSelector(step)
				return nil
			}
		case "enter":
			if m.focus >= formCadence && m.focus <= formStatus {
				m.changeFormSelector(1)
				return nil
			}
			if m.screen == createView && m.focus == formSubjects {
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
	if m.focus < len(m.fields) {
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
	lines := []string{m.list.Styles.Title.Render("Goal"), title, ""}
	labels := []string{"Name", "Hours", "Minutes", "Start date", "End date"}
	for i, label := range labels {
		heading := m.formHeading(i, label)
		if i == formStartDate || i == formEndDate {
			heading += " (optional; e.g. 2026-10-29)"
		}
		lines = append(lines, heading, m.fields[i].View(), "")
	}
	lines = append(lines, m.formHeading(formCadence, "Cadence:")+" "+cadenceLabel(cadenceAt(m.cadence)), "")
	lines = append(lines, m.formHeading(formPriority, "Priority:")+" "+map[string]string{"high": "High", "normal": "Normal", "low": "Low"}[m.priority], "", m.formHeading(formStatus, "Status:")+" "+map[bool]string{true: "Active", false: "Inactive"}[m.isActive], "")
	saveFocus := m.formSaveFocus()
	if m.screen == createView {
		lines = append(lines, m.formHeading(formSubjects, "Subjects:")+fmt.Sprintf(" %d selected (Enter to choose)", len(m.ids())), "")
	}
	lines = append(lines, m.formHeading(saveFocus, "Save"))

	if m.screen == editView && m.selected.TargetSeconds%60 != 0 {
		lines = append(lines, "Current target: "+formatDuration(m.selected.TargetSeconds), "Changing duration uses whole minutes.")
	}
	return strings.Join(lines, "\n")
}
func (m Model) formHeading(index int, label string) string {
	if m.focus == index {
		return m.list.Styles.Title.Background(lipgloss.Color("#FFB000")).Foreground(lipgloss.Color("#1A1A1A")).Render("› " + label)
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
	if !create && m.fields[formHours].Value() == strconv.FormatInt(m.selected.TargetSeconds/3600, 10) && m.fields[formMinutes].Value() == strconv.FormatInt(m.selected.TargetSeconds%3600/60, 10) {
		target = m.selected.TargetSeconds
	} else {
		var err error
		target, err = targetSeconds(m.fields[formHours].Value(), m.fields[formMinutes].Value())
		if err != nil {
			m.status = err.Error()
			return nil
		}
	}

	active := m.isActive
	priority := m.priority
	input := goal.CreateGoalInput{
		Priority:      priority,
		IsActive:      &active,
		Name:          m.fields[formName].Value(),
		TargetSeconds: target,
		Cadence:       cadenceAt(m.cadence),
		StartDate:     m.fields[formStartDate].Value(),
		EndDate:       m.fields[formEndDate].Value(),
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
				Priority:       &priority,
				IsActive:       &active,
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
func (m Model) errorMessage(err error) string {
	if m.screen != createView && errors.Is(err, data.ErrVersionConflict) {
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
	if errors.Is(err, data.ErrDuplicateRecord) {
		return "A Goal with this name already exists."
	}
	if m.screen == createView {
		return "Could not create Goal. Ctrl+S: retry · Esc: cancel."
	}
	if errors.Is(err, data.ErrRecordNotFound) {
		return "Goal or Subject unavailable. R: reload · Esc: cancel."
	}
	return "Could not complete Goal operation. R: retry or reload · Esc: cancel."
}

func (m *Model) changeFormSelector(step int) {
	switch m.focus {
	case formCadence:
		m.cadence = (m.cadence + step + cadenceCount) % cadenceCount
	case formPriority:
		priorities := [...]string{"high", "normal", "low"}
		for index, priority := range priorities {
			if priority == m.priority {
				m.priority = priorities[(index+step+len(priorities))%len(priorities)]
				break
			}
		}
	case formStatus:
		m.isActive = !m.isActive
	}
}
