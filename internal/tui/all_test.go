package tui

import (
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

func TestModel_AllThoughts(t *testing.T) {
	t.Run("uses the same colored heading as the Subjects picker", func(t *testing.T) {
		m := newRootTestModel()
		m.entityList.Select(1)
		m = runModelCommand(t, m, enterKey())
		heading := m.subjects.list.Styles.TitleBar.Render(m.subjects.list.Styles.Title.Render("Thoughts"))
		if !strings.HasPrefix(m.View().Content, heading+"\n") {
			t.Fatalf("got heading %q, want %q", m.View().Content, heading)
		}
	})
	t.Run("late batch cannot replace a reopened list or another thought scope", func(t *testing.T) {
		service := thought.NewService(testutils.NewFakeThoughtStore())
		if _, err := service.Create(t.Context(), "u", "first observation", nil, time.Time{}); err != nil {
			t.Fatal(err)
		}
		m := NewModel(t.Context(), &data.User{UserID: "u"}, &subjectServiceStub{}, service, &metricsStub{}, logging.Nop())
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
		m, cmd := rootUpdate(m, enterKey())
		old := cmd() // Real command result, deliberately held across navigation.
		m, _ = rootUpdate(m, escapeKey())
		if m.screen != screenEntities {
			t.Fatal("Escape did not leave pending list")
		}
		if _, err := service.Create(t.Context(), "u", "newer observation", nil, time.Now().Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
		m, cmd = rootUpdate(m, enterKey())
		m, _ = rootUpdate(m, cmd())
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
		before := m.View().Content
		m, _ = rootUpdate(m, old)
		if m.View().Content != before || !strings.Contains(ansi.Strip(before), "│ Thought 2") {
			t.Fatal("late batch changed active rows or selection")
		}
		m, cmd = rootUpdate(m, enterKey())
		m, _ = rootUpdate(m, cmd())
		if !strings.Contains(m.View().Content, "newer observation") {
			t.Fatal("selected record did not open")
		}
		_, quit := rootUpdate(m, runeKey('q'))
		assertQuitCommand(t, quit)
		m, _ = rootUpdate(m, escapeKey())
		m, _ = rootUpdate(m, escapeKey())
		m.screen = screenMiscThoughts
		cmd = m.thoughts.OpenUnassigned()
		m, _ = rootUpdate(m, cmd())
		before = m.View().Content
		m, _ = rootUpdate(m, old)
		if m.View().Content != before {
			t.Fatal("All Thoughts result changed Misc")
		}
	})
	t.Run("Create treats q as text while control C always quits", func(t *testing.T) {
		m := newRootTestModel()
		m.entityList.Select(1)
		m = runModelCommand(t, m, enterKey())
		m, _ = rootUpdate(m, enterKey())
		m, cmd := rootUpdate(m, runeKey('q'))
		if cmd != nil {
			if _, ok := cmd().(tea.QuitMsg); ok {
				t.Fatal("q quit editor")
			}
		}
		if !strings.Contains(m.View().Content, "q") {
			t.Fatal("q missing from draft")
		}
		_, cmd = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
		assertQuitCommand(t, cmd)
	})
}
