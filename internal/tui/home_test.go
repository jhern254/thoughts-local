package tui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/jhern254/go-thoughts/internal/timeline"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
)

// Existing screen tests begin with Subjects selected in the entity strip.
// Home-specific tests use NewModel directly and exercise its real startup.
func newScreenTestModel(ctx context.Context, user *data.User, subjects SubjectService, thoughtService thoughts.Service, metrics MetricsService, logger logging.Logger) Model {
	m := NewModel(ctx, user, subjects, thoughtService, metrics, &homeEventStub{}, &homeTimelineStub{}, logger)
	m.entityFocused = true
	return m
}

type homeEventStub struct {
	items []data.Event
	user  string
}

func (s *homeEventStub) List(_ context.Context, user string, _, _ time.Time) ([]data.Event, error) {
	s.user = user
	return s.items, nil
}

func (s *homeEventStub) Get(context.Context, string, int64) (*data.Event, error) {
	return &s.items[0], nil
}
func (*homeEventStub) Create(context.Context, string, string, time.Time) (*data.Event, error) {
	panic("unexpected event create")
}
func (*homeEventStub) End(context.Context, string, int64, int64, time.Time) (*data.Event, error) {
	panic("unexpected event end")
}

type homeTimelineStub struct{}

func (*homeTimelineStub) OpenThoughtsView(context.Context, string, int64) (timeline.ThoughtScope, error) {
	panic("unexpected event expansion")
}
func (*homeTimelineStub) BrowseThoughtsView(context.Context, string, timeline.ThoughtScope, data.ThoughtViewRequest) (data.ThoughtView, error) {
	panic("unexpected event thoughts")
}
func (*homeTimelineStub) CountThoughts(context.Context, string, timeline.ThoughtScope) (int64, error) {
	panic("unexpected event count")
}
func (*homeTimelineStub) LatestThought(context.Context, string, int64) (*data.ThoughtSummary, error) {
	panic("unexpected event preview")
}
func (*homeTimelineStub) ThoughtCounts(context.Context, string, time.Time, time.Time) ([]data.EventThoughtCount, error) {
	return nil, nil
}

type homeThoughtStore struct{ summary data.ThoughtSummary }

func (s homeThoughtStore) ListThoughtsInRange(context.Context, string, time.Time, time.Time) ([]data.Thought, error) {
	panic("unexpected full body range")
}
func (s homeThoughtStore) BrowseThoughtsViewInRange(context.Context, string, time.Time, time.Time, data.ThoughtViewRequest) (data.ThoughtView, error) {
	return data.ThoughtView{Items: []data.ThoughtSummary{s.summary}}, nil
}
func (s homeThoughtStore) LatestThoughtInRange(context.Context, string, time.Time, time.Time) (*data.ThoughtSummary, error) {
	return &s.summary, nil
}
func (s homeThoughtStore) CountThoughtsInRange(context.Context, string, time.Time, time.Time) (int64, error) {
	return 1, nil
}
func (s homeThoughtStore) ThoughtCountsByEvent(context.Context, string, time.Time, time.Time, time.Time) ([]data.EventThoughtCount, error) {
	return []data.EventThoughtCount{{EventID: 1, Count: 1}}, nil
}

// Home startup contains one data batch and one minute timer. Only the data
// batch is executed here; timer lifecycle is covered with explicit clock ticks.
func startHomeData(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	return runHomeData(t, m, cmd().(tea.BatchMsg)[0])
}
func runHomeData(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, child := range batch {
			m = runHomeData(t, m, child)
		}
		return m
	}
	m, next := rootUpdate(m, msg)
	return runHomeData(t, m, next)
}

func TestModel_EventsHome(t *testing.T) {
	t.Run("quit works before asynchronous home initialization", func(t *testing.T) {
		m := newScreenTestModel(t.Context(), &data.User{UserID: "u"}, &subjectServiceStub{}, thought.NewService(testutils.NewFakeThoughtStore()), &metricsStub{}, logging.Nop())
		m.entityFocused = false
		_, cmd := m.Update(runeKey('q'))
		assertQuitCommand(t, cmd)
	})
	t.Run("delayed startup does not steal navigation", func(t *testing.T) {
		m := newRootTestModel()
		startup := m.Init()()
		m, _ = rootUpdate(m, enterKey())
		m, cmd := rootUpdate(m, startup)
		if m.screen != screenSubjectList || cmd != nil {
			t.Fatal("startup took over another screen")
		}
	})
	newHome := func(t *testing.T) (Model, *homeEventStub) {
		service := thought.NewService(testutils.NewFakeThoughtStore())
		item, err := service.Create(t.Context(), "home-user", "complete thought detail", nil, time.Now().Add(-time.Minute))
		if err != nil {
			t.Fatal(err)
		}
		events := &homeEventStub{items: []data.Event{{EventID: 1, StartedAt: time.Now().Add(-time.Hour)}}}
		store := homeThoughtStore{summary: data.ThoughtSummary{ThoughtID: item.ThoughtID, Preview: "bounded preview", ObservedAt: item.ObservedAt}}
		m := NewModel(t.Context(), &data.User{UserID: "home-user"}, &subjectServiceStub{list: func(context.Context, string) ([]data.Subject, error) { return nil, nil }}, service, &metricsStub{}, events, timeline.NewService(events, store, store), logging.Nop())
		m, cmd := rootUpdate(m, m.Init()())
		m = startHomeData(t, m, cmd)
		return m, events
	}
	t.Run("boots into owned Events data and wraps horizontal navigation both ways", func(t *testing.T) {
		m, events := newHome(t)
		if events.user != "home-user" || !strings.Contains(m.View().Content, "Events") || !strings.HasPrefix(m.View().Content, "Local user: home-user\n") {
			t.Fatal("home did not use or display bootstrapped user")
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
		for _, key := range []rune{tea.KeyRight, tea.KeyLeft} {
			before := m.entityList.Index()
			m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: key}))
			m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: key}))
			if m.entityList.Index() != before {
				t.Fatal("entity navigation did not wrap")
			}
		}
		m, _ = rootUpdate(m, tea.WindowSizeMsg{Width: 24, Height: 16})
		if !strings.Contains(m.View().Content, "Subjects") {
			t.Fatal("selected entity clipped away")
		}
	})
	t.Run("restores event detail and rejects another thought component's reply", func(t *testing.T) {
		m, _ := newHome(t)
		m, cmd := rootUpdate(m, enterKey())
		m = runHomeData(t, m, cmd)
		before := m.View().Content
		other := m.thoughts.OpenBrowseThoughtsView(&metricsStub{total: 999})
		reply := other().(tea.BatchMsg)[1]()
		m, _ = rootUpdate(m, reply)
		if m.View().Content != before {
			t.Fatal("foreign thought result changed event picker")
		}
		m, cmd = rootUpdate(m, enterKey())
		m = runHomeData(t, m, cmd)
		if !strings.Contains(m.View().Content, "complete thought detail") {
			t.Fatal("did not open shared detail")
		}
		m, _ = rootUpdate(m, escapeKey())
		if m.View().Content != before {
			t.Fatal("return changed event anchors")
		}
		m, _ = rootUpdate(m, escapeKey())
		if strings.Contains(m.View().Content, "r: refresh event") || !strings.Contains(m.View().Content, "Home: first") {
			t.Fatal("one Escape from expanded event should return to timeline navigation")
		}
	})
	t.Run("delayed expansion cannot reopen after leaving and returning home", func(t *testing.T) {
		m, _ := newHome(t)
		m, cmd := rootUpdate(m, enterKey())
		old := cmd()
		m, _ = rootUpdate(m, tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
		m, cmd = rootUpdate(m, enterKey())
		m = runHomeData(t, m, cmd)
		m, cmd = rootUpdate(m, escapeKey())
		m = startHomeData(t, m, cmd)
		before := m.View().Content
		m, _ = rootUpdate(m, old)
		if m.View().Content != before {
			t.Fatal("old expansion changed reopened home")
		}
	})
}
