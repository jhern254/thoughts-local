package goals

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/goal"
)

type services struct {
	item                       data.Goal
	subjects                   []data.Subject
	err                        error
	creates, updates, replaces int
	ids                        []int64
	version                    int64
	input                      goal.UpdateGoalInput
}

func (s *services) List(context.Context, string) ([]data.Goal, error) {
	return []data.Goal{s.item}, s.err
}
func (s *services) ListActive(ctx context.Context, user string) ([]data.Goal, error) {
	return s.List(ctx, user)
}
func (s *services) ListInactive(ctx context.Context, user string) ([]data.Goal, error) {
	return s.List(ctx, user)
}
func (s *services) Get(context.Context, string, int64) (*data.Goal, error) {
	item := s.item
	return &item, s.err
}
func (s *services) Update(_ context.Context, _ string, _ int64, v int64, input goal.UpdateGoalInput) (*data.Goal, error) {
	s.updates++
	s.input = input
	s.version = v
	return &s.item, s.err
}
func (s *services) Create(_ context.Context, _ string, _ goal.CreateGoalInput, ids []int64) (*data.Goal, error) {
	s.creates++
	s.ids = ids
	return &s.item, s.err
}
func (s *services) ReplaceSubjects(_ context.Context, _ string, _ int64, v int64, ids []int64) (*data.Goal, error) {
	s.replaces++
	s.ids = ids
	s.version = v
	return &s.item, s.err
}

type subjectService struct{ s *services }

func (s subjectService) List(context.Context, string) ([]data.Subject, error) {
	return s.s.subjects, s.s.err
}
func (s *services) ListLiveSubjectsForGoal(context.Context, string, int64) ([]data.Subject, error) {
	return s.subjects, s.err
}
func testModel(t *testing.T) (Model, *services) {
	t.Helper()
	s := &services{item: data.Goal{GoalID: 42, GoalName: "Practice Go", TargetSeconds: 5400, Version: 7, TZ: "America/Los_Angeles", WeekStart: "sun", Cadence: "weekly", Priority: "high"}}
	return New(t.Context(), "owner", s, s, s, subjectService{s}), s
}
func press(m Model, key string) (Model, tea.Cmd) {
	switch key {
	case "enter":
		return m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	case "esc":
		return m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	case "tab":
		return m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	case "space":
		return m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	}
	return m.Update(tea.KeyPressMsg{Code: []rune(key)[0], Text: key})
}
func TestModel_Lifecycle(t *testing.T) {
	t.Run("opening loads live memberships and closing rejects the actual reply", func(t *testing.T) {
		m, _ := testModel(t)
		cmd := m.Open()
		reply := cmd()
		m.Close()
		m, _ = m.Update(reply)
		if len(m.list.Items()) != 0 {
			t.Fatal("closed list accepted stale result")
		}
	})
	t.Run("a newer detail request rejects an older result", func(t *testing.T) {
		m, s := testModel(t)
		m.Open()
		m.screen = detailView
		old := m.loadDetail(42)()
		s.item.GoalID = 99
		newer := m.loadDetail(99)()
		m, _ = m.Update(newer)
		m, _ = m.Update(old)
		if m.selected.GoalID != 99 {
			t.Fatalf("selected ID got %d, want 99", m.selected.GoalID)
		}
	})
	t.Run("membership failure is not rendered as Misc", func(t *testing.T) {
		m, s := testModel(t)
		s.err = errors.New("private")
		reply := m.Open()()
		m, _ = m.Update(reply)
		if strings.Contains(m.View(), "Misc") || strings.Contains(m.View(), "private") || m.status == "" {
			t.Fatalf("unsafe failure view: %s", m.View())
		}
	})
}
func TestModel_Editors(t *testing.T) {
	t.Run("property update preserves internal settings and uses the snapshot version", func(t *testing.T) {
		m, s := testModel(t)
		m.open = true
		m.selected = &s.item
		m.startForm(false)
		m.fields[0].SetValue("Updated")
		cmd := m.saveProperties()
		cmd()
		if s.version != 7 || s.input.TZ != s.item.TZ || s.input.WeekStart != "sun" || s.input.Priority != nil || s.input.IsActive != nil || s.replaces != 0 {
			t.Fatalf("update input %+v, version %d", s.input, s.version)
		}
	})
	t.Run("creation uses atomic assignment once while pending", func(t *testing.T) {
		m, s := testModel(t)
		m.open = true
		m.startForm(true)
		m.fields[0].SetValue("New")
		m.fields[1].SetValue("1")
		m.selectedIDs[1] = true
		m.selectedIDs[2] = true
		m, cmd := press(m, "enter")
		_, duplicate := press(m, "enter")
		if duplicate != nil {
			t.Fatal("duplicate save command")
		}
		cmd()
		if s.creates != 1 || len(s.ids) != 2 {
			t.Fatalf("creates %d, IDs %v", s.creates, s.ids)
		}
	})
	t.Run("failed membership save retains the selection", func(t *testing.T) {
		m, s := testModel(t)
		m.open = true
		m.selected = &s.item
		m.screen = subjectsView
		m.pickerReturn = detailView
		m.selectedIDs = map[int64]bool{2: true}
		s.err = data.ErrVersionConflict
		reply := m.saveSubjects()()
		m, _ = m.Update(reply)
		if !m.selectedIDs[2] || m.screen != subjectsView || !strings.Contains(m.status, "Goal changed") {
			t.Fatalf("lost selection or conflict: %+v", m)
		}
	})
	t.Run("cancelling a creation performs no writes", func(t *testing.T) {
		m, s := testModel(t)
		m.open = true
		m.startForm(true)
		m, _ = press(m, "esc")
		if s.creates != 0 || m.screen != listView {
			t.Fatal("cancel created a Goal or failed to return")
		}
	})
}
func TestModel_NarrowForm(t *testing.T) {
	m, _ := testModel(t)
	m.open = true
	m.startForm(true)
	m.Resize(48, 16)
	for range 7 {
		m, _ = press(m, "tab")
	}
	if m.body.YOffset() == 0 {
		t.Fatal("focused Save action was not scrolled into view")
	}
	if !strings.Contains(m.View(), "Save") {
		t.Fatalf("Save not visible: %s", m.View())
	}
}

func TestGoalRow_SubjectPreviews(t *testing.T) {
	for _, count := range []int{0, 1, 3, 4} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			subjects := make([]data.Subject, count)
			for i := range subjects {
				subjects[i] = data.Subject{SubjectID: int64(i + 1), SubjectName: fmt.Sprintf("Subject %d 界", i)}
			}
			row := goalRow{item: data.Goal{GoalName: "Long 界 name", TargetSeconds: 3600, Cadence: "weekly", Priority: "high"}, subjects: subjects}
			description := row.Description()
			if (count == 0) != strings.Contains(description, "Misc") {
				t.Fatalf("Misc preview: %q", description)
			}
			if (count > 3) != strings.Contains(description, "...") {
				t.Fatalf("overflow preview: %q", description)
			}
		})
	}
}
func TestModel_StaleSave(t *testing.T) {
	m, s := testModel(t)
	m.open = true
	m.startForm(true)
	m.fields[0].SetValue("New")
	m.fields[1].SetValue("1")
	cmd := m.saveProperties()
	reply := cmd()
	m.Close()
	m.Open()
	m, _ = m.Update(reply)
	if m.screen != listView || m.selected != nil {
		t.Fatal("retired save replaced newer screen")
	}
	if s.creates != 1 {
		t.Fatal("expected submitted creation")
	}
}

func TestModel_DetailScroll(t *testing.T) {
	t.Run("opening Details starts at its heading after a scrolled view", func(t *testing.T) {
		m, s := testModel(t)
		m.open = true
		m.screen = detailView
		m.Resize(80, 24)
		m.body.SetContent(strings.Repeat("old content\n", 100))
		m.body.SetYOffset(50)
		for i := range 100 {
			s.subjects = append(s.subjects, data.Subject{SubjectID: int64(i + 1), SubjectName: "Long membership"})
		}
		reply := m.loadDetail(s.item.GoalID)()
		m, _ = m.Update(reply)
		if !strings.HasPrefix(m.View(), s.item.GoalName) {
			t.Fatalf("Details heading hidden by previous scroll: %s", m.View())
		}
	})
}

func TestModel_ResizeFocus(t *testing.T) {
	m, _ := testModel(t)
	m.open = true
	m.Resize(100, 40)
	m.startForm(true)
	for range 7 {
		m, _ = press(m, "tab")
	}
	m.Resize(48, 16)
	if !strings.Contains(m.View(), "Save") {
		t.Fatalf("resize hid the focused Save action: %s", m.View())
	}
}
func TestGoalRow_DateBounds(t *testing.T) {
	start, end := "2026-01-01", "2026-12-31"
	description := (goalRow{item: data.Goal{StartDate: &start, EndDate: &end}}).Description()
	if strings.Count(description, "\n") > 2 || !strings.Contains(description, start) || !strings.Contains(description, end) {
		t.Fatalf("date bounds exceed the three description rows: %q", description)
	}
}

func TestModel_SubjectReadFailure(t *testing.T) {
	m, s := testModel(t)
	m.open = true
	m.selected = &s.item
	m.screen = subjectsView
	m.pickerReturn = detailView
	s.err = errors.New("Subject list unavailable")
	reply := m.loadPicker()()
	m, _ = m.Update(reply)
	_, save := press(m, "enter")
	if save != nil {
		t.Fatal("failed Subject load enabled a save that could clear unseen memberships")
	}
	if s.replaces != 0 {
		t.Fatal("failed Subject load changed memberships")
	}
}

func TestModel_DetailReadFailure(t *testing.T) {
	m, s := testModel(t)
	m.open = true
	m.Resize(80, 24)
	m.screen = detailView
	m.selected = &s.item
	m.live = []data.Subject{{SubjectName: "Previous membership"}}
	s.err = data.ErrRecordNotFound
	reply := m.loadDetail(99)()
	m, _ = m.Update(reply)
	m, _ = press(m, "e")
	if m.screen != detailView || m.selected != nil || strings.Contains(m.View(), "Previous membership") || strings.Contains(m.View(), "Misc") {
		t.Fatal("failed new Details read exposed a previous Goal as editable")
	}
}

func TestModel_ExistingTargetPrecision(t *testing.T) {
	for _, seconds := range []int64{30, 5430} {
		t.Run(fmt.Sprint(seconds), func(t *testing.T) {
			m, s := testModel(t)
			s.item.TargetSeconds = seconds
			m.open = true
			m.selected = &s.item
			m.startForm(false)
			m.fields[0].SetValue("Renamed")
			cmd := m.saveProperties()
			if cmd == nil {
				t.Fatalf("untouched target prevented rename: %s", m.status)
			}
			cmd()
			if s.input.TargetSeconds != seconds {
				t.Fatalf("target got %d, want %d", s.input.TargetSeconds, seconds)
			}
		})
	}
}
