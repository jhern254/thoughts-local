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
