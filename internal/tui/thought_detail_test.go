package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
)

// The SQLite browse projection supplies joined subject labels. This fixture
// supplies that same label for its single-subject collection.
type labeledThoughtService struct {
	*thought.Service
	name *string
}

func (s labeledThoughtService) BrowseView(ctx context.Context, userID string, request data.ThoughtViewRequest) (data.ThoughtView, error) {
	view, err := s.Service.BrowseView(ctx, userID, request)
	for i := range view.Items {
		view.Items[i].SubjectName = s.name
	}
	return view, err
}

func TestModel_SharedThoughtDetail(t *testing.T) {
	for _, scope := range []string{"subject", "Misc"} {
		t.Run(scope+" and the thought browse view show identical detail and restore their lists", func(t *testing.T) {
			subjects := subject.NewService(testutils.NewFakeSubjectStore())
			parent, err := subjects.Create(t.Context(), "u", "Writing")
			if err != nil {
				t.Fatal(err)
			}
			var subjectID *int64
			var subjectName *string
			heading := "Misc"
			if scope == "subject" {
				subjectID = &parent.SubjectID
				subjectName = &parent.SubjectName
				heading = parent.SubjectName
			}
			thoughts := thought.NewService(testutils.NewFakeThoughtStore())
			for i := range 30 {
				body := fmt.Sprintf("Thought body %d\n", i) + strings.Repeat("Full detail line\n", 30)
				if _, err := thoughts.Create(t.Context(), "u", body, subjectID, time.Unix(1700000000+int64(i), 0)); err != nil {
					t.Fatal(err)
				}
			}
			var details []string
			for _, browseThoughtsView := range []bool{false, true} {
				m := NewModel(t.Context(), &data.User{UserID: "u"}, subjects, labeledThoughtService{Service: thoughts, name: subjectName}, &metricsStub{}, logging.Nop())
				m, _ = rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 24})
				if browseThoughtsView {
					m.entityList.Select(1)
					m = runModelCommand(t, m, enterKey())
				} else {
					m = openSubjects(t, m)
					if scope == "subject" {
						m = rootOpenThoughts(t, m)
					} else {
						m.subjects.list.Select(1)
						m = runModelCommand(t, m, enterKey())
					}
				}
				for range 10 {
					m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
				}
				listView, origin := m.View().Content, m.screen
				m = runModelCommand(t, m, enterKey())
				wantHeading := m.subjects.list.Styles.Title.Render(heading) + "\n\n"
				if !strings.HasPrefix(m.View().Content, wantHeading) {
					t.Fatalf("got detail %q, want styled subject heading %q", m.View().Content, wantHeading)
				}
				details = append(details, m.View().Content)
				m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyPgDown}))
				m, _ = rootUpdate(m, escapeKey())
				if m.screen != origin || m.View().Content != listView {
					t.Fatal("detail return changed the originating list, selection or scroll position")
				}
			}
			if details[0] != details[1] {
				t.Fatalf("detail differs by route:\n%s\nversus:\n%s", details[0], details[1])
			}
			if !strings.Contains(details[0], "Thought 21 • ") || !strings.Contains(details[0], "Thought body 20") {
				t.Fatalf("expected standalone thought detail, got %q", details[0])
			}
		})
	}
}

func TestModel_ThoughtDetailMutations(t *testing.T) {
	for _, route := range []string{"subject", "Misc", "All Thoughts"} {
		t.Run(route+" keeps the shared heading and owns edit/delete keys", func(t *testing.T) {
			subjects := subject.NewService(testutils.NewFakeSubjectStore())
			parent, err := subjects.Create(t.Context(), "u", "Writing")
			if err != nil {
				t.Fatal(err)
			}
			var subjectID *int64
			var subjectName *string
			heading := "Misc"
			if route != "Misc" {
				subjectID = &parent.SubjectID
				subjectName = &parent.SubjectName
				heading = "Writing"
			}
			service := thought.NewService(testutils.NewFakeThoughtStore())
			item, err := service.Create(t.Context(), "u", "original", subjectID, time.Unix(1700000000, 0))
			if err != nil {
				t.Fatal(err)
			}
			m := NewModel(t.Context(), &data.User{UserID: "u"}, subjects, labeledThoughtService{Service: service, name: subjectName}, &metricsStub{}, logging.Nop())
			if route == "All Thoughts" {
				m.entityList.Select(1)
				m = runModelCommand(t, m, enterKey())
			} else {
				m = openSubjects(t, m)
				if route == "subject" {
					m = rootOpenThoughts(t, m)
				} else {
					m.subjects.list.Select(1)
					m = runModelCommand(t, m, enterKey())
				}
			}
			m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
			m = runModelCommand(t, m, enterKey())
			origin := m.screen
			m, _ = rootUpdate(m, runeKey('e'))
			prefix := m.subjects.list.Styles.Title.Render(heading) + "\n\n"
			if !strings.HasPrefix(m.View().Content, prefix) || !strings.Contains(m.View().Content, "Edit thought") {
				t.Fatalf("got editor %q, want shared heading %q", m.View().Content, prefix)
			}
			m, _ = rootUpdate(m, tea.PasteMsg{Content: " edited"})
			m, cmd := rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
			_, quit := rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
			if quit == nil {
				t.Fatal("pending edit blocked Ctrl+C")
			}
			if _, ok := quit().(tea.QuitMsg); !ok {
				t.Fatal("Ctrl+C did not quit")
			}
			m, changed := rootUpdate(m, cmd())
			m = applyCommand(t, m, changed)
			got, err := service.Get(t.Context(), "u", item.ThoughtID)
			if err != nil || got.Thought != "original edited" || got.Version != 2 {
				t.Fatalf("got edited thought %v/%v, want saved body and version 2", got, err)
			}
			if !m.subjects.listStale || m.screen != origin {
				t.Fatal("update lost origin or count invalidation")
			}
			m = runModelCommand(t, m, escapeKey())
			m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
			m = runModelCommand(t, m, enterKey())
			m, _ = rootUpdate(m, runeKey('d'))
			if !strings.HasPrefix(m.View().Content, prefix) || !strings.Contains(m.View().Content, "Delete thought") {
				t.Fatal("thought delete opened wrong presentation")
			}
			m, cmd = rootUpdate(m, runeKey('y'))
			_, quit = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
			if quit == nil {
				t.Fatal("pending delete blocked Ctrl+C")
			}
			m, next := rootUpdate(m, cmd())
			m = applyCommand(t, m, next)
			if !m.thoughts.Browsing() || m.screen != origin || strings.Contains(m.View().Content, "original edited") {
				t.Fatal("deletion did not return to refreshed origin")
			}
			if _, err := subjects.Get(t.Context(), "u", parent.SubjectID); err != nil {
				t.Fatalf("thought delete changed subject: %v", err)
			}
		})
	}
}
