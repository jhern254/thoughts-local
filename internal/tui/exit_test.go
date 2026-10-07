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
	"github.com/jhern254/go-thoughts/internal/timeline"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
)

func controlC() tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl})
}

func TestModel_ExitConfirmation(t *testing.T) {
	for _, focused := range []bool{false, true} {
		for _, key := range []tea.KeyPressMsg{controlC(), escapeKey()} {
			t.Run(key.String()+" confirms exit with entity focus "+map[bool]string{false: "off", true: "on"}[focused], func(t *testing.T) {
				m := newRootTestModel()
				m.entityFocused = focused
				m.events.SetFocused(!focused)
				before := m.View().Content
				m, cmd := rootUpdate(m, key)
				if cmd != nil || !strings.Contains(ansi.Strip(m.View().Content), "> No") {
					t.Fatal("exit request should show confirmation with No selected and no command")
				}
				m, cmd = rootUpdate(m, enterKey())
				if cmd != nil || m.View().Content != before || m.entityFocused != focused {
					t.Fatal("default No should restore the same home and panel focus")
				}
				m, _ = rootUpdate(m, key)
				_, cmd = rootUpdate(m, runeKey('y'))
				assertQuitCommand(t, cmd)
			})
		}
	}
	t.Run("selection keys accept Yes and reopening selects No", func(t *testing.T) {
		for _, key := range []tea.KeyPressMsg{
			{Code: tea.KeyLeft}, {Code: tea.KeyRight}, {Code: tea.KeyTab}, {Code: tea.KeyTab, Mod: tea.ModShift},
		} {
			m := newRootTestModel()
			m, _ = rootUpdate(m, controlC())
			m, _ = rootUpdate(m, key)
			if !strings.Contains(ansi.Strip(m.View().Content), "> Yes") {
				t.Fatalf("%s did not select Yes", key.String())
			}
			_, cmd := rootUpdate(m, enterKey())
			assertQuitCommand(t, cmd)
			m, _ = rootUpdate(m, runeKey('q'))
			m, _ = rootUpdate(m, controlC())
			if !strings.Contains(ansi.Strip(m.View().Content), "> No") {
				t.Fatal("reopening retained Yes")
			}
		}
	})
	t.Run("cancel keys restore navigation and other interactions stay inside confirmation", func(t *testing.T) {
		for _, cancel := range []tea.KeyPressMsg{escapeKey(), runeKey('q'), runeKey('n')} {
			m := newRootTestModel()
			m.browserRecordingEnabled = true
			before := m.View().Content
			m, _ = rootUpdate(m, controlC())
			for _, message := range []tea.Msg{controlC(), runeKey('t'), runeKey('e'), thoughts.VoiceAction{Action: "open"}, tea.PasteMsg{Content: "y"}} {
				var cmd tea.Cmd
				m, cmd = rootUpdate(m, message)
				if cmd != nil || !strings.Contains(m.View().Content, "Exit app?") || m.VoiceState().CanOpenThoughtDraft {
					t.Fatalf("%T escaped the confirmation", message)
				}
			}
			m, cmd := rootUpdate(m, cancel)
			if cmd != nil || m.View().Content != before {
				t.Fatalf("%s did not restore navigation", cancel.String())
			}
		}
	})
	t.Run("startup and asynchronous data cannot dismiss confirmation", func(t *testing.T) {
		m := newRootTestModel()
		m, _ = rootUpdate(m, controlC())
		m, cmd := rootUpdate(m, m.Init()())
		m = startHomeData(t, m, cmd)
		if !strings.Contains(m.View().Content, "Exit app?") {
			t.Fatal("startup data dismissed confirmation")
		}
		m, _ = rootUpdate(m, runeKey('n'))
		if strings.Contains(m.View().Content, "Exit app?") || !strings.Contains(m.View().Content, "Events") {
			t.Fatal("cancel did not reveal updated home")
		}
	})
	t.Run("confirmation fits normal and narrow terminals and survives resize", func(t *testing.T) {
		m := newRootTestModel()
		m.browserRecordingEnabled = true
		m, _ = rootUpdate(m, controlC())
		for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 40, Height: 16}} {
			m, _ = rootUpdate(m, size)
			view := ansi.Strip(m.View().Content)
			if len(strings.Split(view, "\n")) > size.Height || !strings.Contains(view, "> No") || !strings.Contains(view, "Enter") {
				t.Fatalf("confirmation does not fit %dx%d: %q", size.Width, size.Height, view)
			}
			for _, row := range strings.Split(view, "\n") {
				if ansi.StringWidth(row) > size.Width {
					t.Fatalf("row %q exceeds %d terminal cells", row, size.Width)
				}
			}
		}
		m, _ = rootUpdate(m, runeKey('n'))
		if !strings.Contains(m.View().Content, "Esc/Ctrl+C: exit") || !strings.Contains(m.View().Content, "Enter: open") {
			t.Fatal("narrow entity selector hides exit or entry keys")
		}
	})
}

func TestModel_BackKeys(t *testing.T) {
	t.Run("Q leaves entity selection but does nothing on top-level Events", func(t *testing.T) {
		m := newRootTestModel()
		m, cmd := rootUpdate(m, runeKey('q'))
		if cmd != nil || m.entityFocused || m.screen != screenEvents {
			t.Fatal("Q should focus Events without quitting")
		}
		before := m.View().Content
		m, cmd = rootUpdate(m, runeKey('q'))
		if cmd != nil || m.View().Content != before {
			t.Fatal("Q should do nothing at top-level Events")
		}
	})
	for _, key := range []tea.KeyPressMsg{runeKey('q'), escapeKey()} {
		t.Run(key.String()+" follows subject and thought hierarchy", func(t *testing.T) {
			m := rootOpenThoughts(t, filterRoot(t))
			m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
			m = runModelCommand(t, m, enterKey())
			m, _ = rootUpdate(m, controlC())
			if !m.thoughts.ShowingDetail() || strings.Contains(m.View().Content, "Exit app?") {
				t.Fatal("Ctrl+C changed a nested detail")
			}
			m, reload := rootUpdate(m, runeKey('r'))
			late := reload()
			m, _ = rootUpdate(m, key)
			if !m.thoughts.Browsing() || m.screen != screenSubjectDetail {
				t.Fatal("back should return to this subject's thought list during reload")
			}
			m, _ = rootUpdate(m, late)
			if !m.thoughts.Browsing() {
				t.Fatal("late detail reply reopened detail")
			}
			m, _ = rootUpdate(m, key)
			if m.screen != screenSubjectList {
				t.Fatal("back should return to Subjects")
			}
			m, _ = rootUpdate(m, key)
			if m.screen != screenEvents || !m.entityFocused {
				t.Fatal("back should return to entity selection")
			}
		})
	}
	t.Run("Q clears an applied filter before leaving Subjects", func(t *testing.T) {
		m := filterRoot(t)
		m, _ = rootUpdate(m, runeKey('/'))
		if strings.Contains(m.View().Content, "Q/Esc:") {
			t.Fatal("subject filter input advertises Q as navigation")
		}
		m, cmd := rootUpdate(m, tea.PasteMsg{Content: "alpha"})
		m, _ = rootUpdate(m, rootFilterReply(t, cmd))
		m, _ = rootUpdate(m, enterKey())
		if !strings.Contains(m.View().Content, "Q/Esc: clear filter") {
			t.Fatal("applied subject filter does not advertise its back behavior")
		}
		m, cmd = rootUpdate(m, runeKey('q'))
		if cmd != nil || m.screen != screenSubjectList || m.subjects.list.IsFiltered() {
			t.Fatal("Q should clear the applied filter without quitting or leaving Subjects")
		}
	})
	for _, state := range []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"editor", enterKey()}, {"filter", runeKey('/')},
	} {
		t.Run("Q remains text in thought "+state.name+" and Ctrl+C is ignored", func(t *testing.T) {
			m := rootOpenThoughts(t, filterRoot(t))
			m, _ = rootUpdate(m, state.key)
			m, _ = rootUpdate(m, runeKey('q'))
			before := m.View().Content
			m, cmd := rootUpdate(m, controlC())
			if cmd != nil || m.View().Content != before || !strings.Contains(before, "q") {
				t.Fatal("Ctrl+C changed input or Q was not text")
			}
		})
	}
}

func TestModel_ExitBoundaries(t *testing.T) {
	newHome := func(t *testing.T) Model {
		t.Helper()
		service := thought.NewService(testutils.NewFakeThoughtStore())
		at := time.Now().Add(-time.Minute)
		item, err := service.Create(t.Context(), "u", "complete event thought", nil, at)
		if err != nil {
			t.Fatal(err)
		}
		eventService := &homeEventStub{items: []data.Event{{EventID: 1, StartedAt: at.Add(-time.Minute)}}}
		store := homeThoughtStore{summary: data.ThoughtSummaryView{ThoughtID: item.ThoughtID, Preview: "event preview", ObservedAt: at}}
		m := NewModel(t.Context(), &data.User{UserID: "u"}, &subjectServiceStub{}, service, &metricsStub{}, eventService, timeline.NewService(eventService, store, store), logging.Nop())
		m, cmd := rootUpdate(m, m.Init()())
		return startHomeData(t, m, cmd)
	}
	for _, key := range []tea.KeyPressMsg{runeKey('q'), escapeKey()} {
		t.Run(key.String()+" returns through event detail and expansion before requesting exit", func(t *testing.T) {
			m := newHome(t)
			m, cmd := rootUpdate(m, enterKey())
			m = runHomeData(t, m, cmd)
			before := m.View().Content
			m, cmd = rootUpdate(m, controlC())
			if cmd != nil || m.View().Content != before {
				t.Fatal("Ctrl+C changed an expanded card")
			}
			m, cmd = rootUpdate(m, enterKey())
			m = runHomeData(t, m, cmd)
			if !strings.Contains(m.View().Content, "complete event thought") {
				t.Fatal("event thought did not open")
			}
			before = m.View().Content
			m, cmd = rootUpdate(m, controlC())
			if cmd != nil || m.View().Content != before {
				t.Fatal("Ctrl+C changed event thought detail")
			}
			m, cmd = rootUpdate(m, key)
			if cmd != nil || !strings.Contains(m.View().Content, "event preview") || m.events.AtTopLevel() {
				t.Fatal("back did not restore expanded event")
			}
			m, cmd = rootUpdate(m, key)
			if cmd != nil || !m.events.AtTopLevel() || strings.Contains(m.View().Content, "Exit app?") {
				t.Fatal("back should collapse event without opening exit confirmation")
			}
			before = m.View().Content
			m, _ = rootUpdate(m, controlC())
			m, _ = rootUpdate(m, runeKey('n'))
			if m.View().Content != before {
				t.Fatal("cancel changed the populated timeline's selection or scroll position")
			}
		})
		t.Run(key.String()+" leaves curve controls before requesting exit", func(t *testing.T) {
			m := newHome(t)
			m, _ = rootUpdate(m, runeKey('d'))
			before := m.View().Content
			m, cmd := rootUpdate(m, controlC())
			if cmd != nil || m.View().Content != before {
				t.Fatal("Ctrl+C changed curve controls")
			}
			m, cmd = rootUpdate(m, key)
			if cmd != nil || !m.events.AtTopLevel() || strings.Contains(m.View().Content, "Exit app?") {
				t.Fatal("back did not leave curve controls")
			}
		})
	}
	t.Run("Ctrl+C is ignored in event forms and Q remains text", func(t *testing.T) {
		m := newHome(t)
		m, _ = rootUpdate(m, runeKey('n'))
		m, _ = rootUpdate(m, runeKey('q'))
		before := m.View().Content
		m, cmd := rootUpdate(m, controlC())
		if cmd != nil || m.View().Content != before || !m.events.FormOpen() {
			t.Fatal("Ctrl+C changed an event form")
		}
		m, _ = rootUpdate(m, escapeKey())
		if m.events.FormOpen() || strings.Contains(m.View().Content, "Exit app?") {
			t.Fatal("Esc should cancel event form without requesting exit")
		}
	})
	t.Run("Ctrl+C cannot quit subject screens and Q cancels deletion", func(t *testing.T) {
		m := rootOpenThoughts(t, filterRoot(t))
		for _, screen := range []screen{screenSubjectList, screenSubjectDetail, screenSubjectCreate, screenSubjectEdit, screenSubjectDelete, screenMiscThoughts, screenBrowseThoughts, screenVoiceThought} {
			m.screen = screen
			before := m.View().Content
			updated, cmd := rootUpdate(m, controlC())
			if cmd != nil || updated.View().Content != before {
				t.Fatalf("Ctrl+C changed screen %v", screen)
			}
		}
		m.screen = screenSubjectDelete
		m, cmd := rootUpdate(m, runeKey('q'))
		if cmd != nil || m.screen != screenSubjectDetail {
			t.Fatal("Q should cancel deletion")
		}
	})
}
