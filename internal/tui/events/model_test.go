package events

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
)

type eventStub struct {
	items                []data.Event
	lists, creates, ends int
	start                time.Time
	label                string
	err                  error
}

func (s *eventStub) List(context.Context, string, time.Time, time.Time) ([]data.Event, error) {
	s.lists++
	return s.items, nil
}
func (s *eventStub) Get(_ context.Context, _ string, id int64) (*data.Event, error) {
	for _, item := range s.items {
		if item.EventID == id {
			return &item, nil
		}
	}
	return nil, data.ErrRecordNotFound
}
func (s *eventStub) Create(_ context.Context, _ string, label string, start time.Time) (*data.Event, error) {
	s.creates++
	s.start = start
	s.label = label
	return &data.Event{EventID: 9, StartedAt: start}, s.err
}
func (s *eventStub) End(_ context.Context, _ string, id, version int64, end time.Time) (*data.Event, error) {
	s.ends++
	return &data.Event{EventID: id, Version: version + 1, EndedAt: &end}, s.err
}

type viewStore struct {
	items         []data.ThoughtSummary
	counts, reads int
	err           error
}

func (s *viewStore) ListThoughtsInRange(context.Context, string, time.Time, time.Time) ([]data.Thought, error) {
	panic("full bodies requested for previews")
}
func (s *viewStore) BrowseThoughtsViewInRange(_ context.Context, _ string, from, until time.Time, request data.ThoughtViewRequest) (data.ThoughtView, error) {
	s.reads++
	items := []data.ThoughtSummary{}
	for _, item := range s.items {
		if item.ObservedAt.Before(from) || !item.ObservedAt.Before(until) {
			continue
		}
		if request.Cursor != nil {
			if request.Direction == data.ThoughtsNewer && item.ThoughtID <= request.Cursor.ThoughtID {
				continue
			}
			if request.Direction == data.ThoughtsOlder && item.ThoughtID >= request.Cursor.ThoughtID {
				continue
			}
		}
		items = append(items, item)
	}
	more := len(items) > 50
	if more {
		if request.Direction == data.ThoughtsNewer {
			items = items[:50]
		} else {
			items = items[len(items)-50:]
		}
	}
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	return data.ThoughtView{Items: items, More: more}, nil
}
func (s *viewStore) LatestThoughtInRange(context.Context, string, time.Time, time.Time) (*data.ThoughtSummary, error) {
	if len(s.items) == 0 {
		return nil, nil
	}
	return &s.items[len(s.items)-1], nil
}
func (s *viewStore) ThoughtCountsByEvent(context.Context, string, time.Time, time.Time, time.Time) ([]data.EventThoughtCount, error) {
	s.counts++
	return []data.EventThoughtCount{{EventID: 1, Count: int64(len(s.items))}}, s.err
}
func (s *viewStore) CountThoughtsInRange(context.Context, string, time.Time, time.Time) (int64, error) {
	s.counts++
	return int64(len(s.items)), s.err
}

func TestModel_DayArrival(t *testing.T) {
	t.Run("previous day opens at first event without entering its thought picker", func(t *testing.T) {
		m, service, _ := fixture(t)
		start := m.day.AddDate(0, 0, -1).Add(21 * time.Hour)
		end := start.Add(time.Hour)
		service.items = []data.Event{{EventID: 2, StartedAt: start, EndedAt: &end}}
		m, cmd := m.Update(eventKey("left"))
		m = execute(t, m, cmd)
		if m.inside || m.expanded != 0 || m.index != 0 || !strings.HasPrefix(strings.Split(ansi.Strip(m.View()), "\n")[2], "09:00 PM  ╭") {
			t.Fatalf("got day view %q, want first event visible with calendar focus", ansi.Strip(m.View()))
		}
	})
}

func eventKey(name string) tea.KeyPressMsg {
	switch name {
	case "left":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyLeft})
	case "right":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyRight})
	case "enter":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter})
	case "esc":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape})
	case "up":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyUp})
	case "down":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyDown})
	case "home":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyHome})
	case "end":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyEnd})
	case "tab":
		return tea.KeyPressMsg(tea.Key{Code: tea.KeyTab})
	}
	return tea.KeyPressMsg(tea.Key{Code: rune(name[0]), Text: name})
}

// Execute ordinary data commands and batches; Open's timer is deliberately
// not executed in fixtures. Clock tests deliver ticks without sleeping.
func execute(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, child := range batch {
			m = execute(t, m, child)
		}
		return m
	}
	m, next := m.Update(msg)
	return execute(t, m, next)
}
func fixture(t *testing.T) (Model, *eventStub, *viewStore) {
	t.Helper()
	at := time.Date(2026, 9, 11, 18, 37, 0, 0, time.UTC)
	label := "Reading"
	s := &eventStub{items: []data.Event{{EventID: 1, StartedAt: at.Add(-time.Hour), ActivityType: &label, Version: 1}}}
	v := &viewStore{}
	store := testutils.NewFakeThoughtStore()
	service := thought.NewService(store)
	for i := 0; i < 230; i++ {
		item, err := service.Create(t.Context(), "u", fmt.Sprintf("preview %03d full content", i), nil, at.Add(-time.Hour).Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		v.items = append(v.items, data.ThoughtSummary{ThoughtID: item.ThoughtID, Preview: fmt.Sprintf("preview %03d", i), ObservedAt: item.ObservedAt, CreatedAt: item.CreatedAt})
	}
	m := New(t.Context(), "u", s, timeline.NewService(s, v, v), service, logging.Nop())
	m.now = func() time.Time { return at }
	m.clock = at
	m.day = displaytime.Day(at)
	m.active = true
	m.Resize(80, 27)
	m = execute(t, m, m.reload())
	return m, s, v
}

func TestModel_EventThoughtPicker(t *testing.T) {
	t.Run("starts newest at top and scrolls beyond the bounded window in both directions", func(t *testing.T) {
		m, _, v := fixture(t)
		m, cmd := m.Update(eventKey("enter"))
		m = execute(t, m, cmd)
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "230 thoughts · Newest first") || !strings.Contains(view, "preview 229") || strings.Contains(view, "Create thought") {
			t.Fatalf("unexpected expanded view:\n%s", view)
		}
		if strings.Index(view, "preview 229") > strings.Index(view, "preview 228") || !strings.Contains(view, "││  Misc  preview 229") {
			t.Fatal("newest thought is not first and selected")
		}
		counts := v.counts
		for range 210 {
			m, cmd = m.Update(eventKey("down"))
			m = execute(t, m, cmd)
		}
		if v.counts != counts || !strings.Contains(ansi.Strip(m.View()), "preview 019") {
			t.Fatal("older scrolling lost selection or recounted")
		}
		for range 210 {
			m, cmd = m.Update(eventKey("up"))
			m = execute(t, m, cmd)
		}
		if !strings.Contains(ansi.Strip(m.View()), "preview 229") || v.counts != counts {
			t.Fatal("newer scrolling lost latest selection or recounted")
		}
		if v.reads < 5 {
			t.Fatal("did not exercise cursor batches")
		}
	})
	t.Run("resizes complete previews and restores both anchors after shared detail", func(t *testing.T) {
		m, _, v := fixture(t)
		m, cmd := m.Update(eventKey("enter"))
		m = execute(t, m, cmd)
		short := strings.Count(ansi.Strip(m.View()), "Thought ")
		reads := v.reads
		m.Resize(100, 42)
		tall := strings.Count(ansi.Strip(m.View()), "Thought ")
		if tall <= short || v.reads != reads {
			t.Fatal("resize did not fit more rows or queried data")
		}
		before := m.View()
		offset := m.offset
		m, cmd = m.Update(eventKey("enter"))
		m = execute(t, m, cmd)
		if !m.picker.ShowingDetail() || !strings.Contains(m.View(), "full content") {
			t.Fatal("did not use full thought detail")
		}
		m, cmd = m.Update(eventKey("esc"))
		m = execute(t, m, cmd)
		if m.View() != before || m.offset != offset {
			t.Fatal("detail return lost anchors")
		}
		m, _ = m.Update(eventKey("esc"))
		if m.inside || m.expanded != 0 {
			t.Fatal("one Escape should collapse inline picker")
		}
	})
	t.Run("refresh retains a selected thought still in the latest batch", func(t *testing.T) {
		m, _, _ := fixture(t)
		m, cmd := m.Update(eventKey("enter"))
		m = execute(t, m, cmd)
		m, _ = m.Update(eventKey("down"))
		m, _ = m.Update(eventKey("down"))
		before := m.View()
		m, cmd = m.Update(eventKey("r"))
		m = execute(t, m, cmd)
		if m.View() != before {
			t.Fatalf("refresh changed view:\n%s\nwant:\n%s", ansi.Strip(m.View()), ansi.Strip(before))
		}
	})
}

func TestModel_ClockAndOwnership(t *testing.T) {
	t.Run("next day stops at today without fetching future data", func(t *testing.T) {
		m, s, _ := fixture(t)
		today, calls := m.day, s.lists
		items := m.items
		m.items = nil
		before := m.View()
		m, cmd := m.Update(eventKey("right"))
		if cmd != nil || m.View() != before || s.lists != calls {
			t.Fatal("next day changed today's view or requested future data")
		}
		m.items = items
		m, cmd = m.Update(eventKey("left"))
		m = execute(t, m, cmd)
		if !m.day.Equal(today.AddDate(0, 0, -1)) {
			t.Fatal("previous day unavailable")
		}
		m, cmd = m.Update(eventKey("right"))
		m = execute(t, m, cmd)
		if !m.day.Equal(today) || s.lists != calls+2 {
			t.Fatal("could not navigate back to today")
		}
		m, cmd = m.Update(eventKey("h"))
		m = execute(t, m, cmd)
		if !m.day.Equal(today.AddDate(0, 0, -1)) {
			t.Fatal("h did not navigate to the previous day")
		}
		m, cmd = m.Update(eventKey("l"))
		m = execute(t, m, cmd)
		if !m.day.Equal(today) || s.lists != calls+4 {
			t.Fatal("l did not navigate back to today")
		}
	})
	t.Run("ticks update time without reads and navigation pauses following", func(t *testing.T) {
		m, s, v := fixture(t)
		lists, counts, reads := s.lists, v.counts, v.reads
		m, _ = m.Update(Tick{m.owner, m.session, m.clock.Add(time.Minute)})
		if !strings.Contains(ansi.Strip(m.View()), "1h 1m") || s.lists != lists || v.counts != counts || v.reads != reads {
			t.Fatal("clock failed or queried data")
		}
		m, _ = m.Update(eventKey("home"))
		offset := m.offset
		m, _ = m.Update(Tick{m.owner, m.session, m.clock.Add(time.Minute)})
		if m.following || m.offset != offset {
			t.Fatal("tick moved manual navigation")
		}
		m, _ = m.Update(eventKey("end"))
		if !m.following {
			t.Fatal("End did not resume")
		}
	})
	t.Run("old day and expansion results cannot change a reopened session", func(t *testing.T) {
		m, _, _ := fixture(t)
		cmd := m.reload()
		batch := cmd().(tea.BatchMsg)
		old := batch[0]()
		oldCount := batch[1]()
		oldTick := Tick{m.owner, m.session, m.clock.Add(time.Minute)}
		m, cmd = m.Update(eventKey("enter"))
		opened := cmd()
		m.Close()
		m.active = true
		m.request++
		m.expansion++
		before := m.View()
		m, _ = m.Update(old)
		m, _ = m.Update(oldCount)
		m, tick := m.Update(oldTick)
		if tick != nil {
			t.Fatal("obsolete timer restarted a chain")
		}
		m, _ = m.Update(opened)
		if m.View() != before {
			t.Fatal("stale reply changed session")
		}
	})
	t.Run("count failure remains safe and leaves browsing available", func(t *testing.T) {
		m, _, v := fixture(t)
		v.err = fmt.Errorf("PRIVATE_MARKER: %w", data.ErrDatabaseBusy)
		var logs bytes.Buffer
		logger, err := logging.New(&logs, "test", "info")
		if err != nil {
			t.Fatal(err)
		}
		m.logger = logger
		m = execute(t, m, m.reload())
		view := m.View()
		if !strings.Contains(view, "Thought count unavailable") || strings.Contains(view, "PRIVATE_MARKER") || strings.Contains(logs.String(), "PRIVATE_MARKER") || !strings.Contains(logs.String(), "database_busy") || !errors.Is(m.countErr, data.ErrDatabaseBusy) {
			t.Fatal("unsafe or swallowed count failure")
		}
		var entry map[string]any
		if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
			t.Fatal(err)
		}
		allowed := map[string]bool{"time": true, "caller": true, "application": true, "level": true, "operation": true, "category": true, "message": true}
		for field := range entry {
			if !allowed[field] {
				t.Fatalf("unapproved event field %q", field)
			}
		}
		if len(entry) != len(allowed) || entry["operation"] != "event_thought_count" || entry["category"] != "database_busy" {
			t.Fatalf("unexpected fixed metadata: %v", entry)
		}
	})
}

func TestModel_EventForms(t *testing.T) {
	t.Run("creating after scrolling selects the new event before the next clock tick", func(t *testing.T) {
		m, s, _ := fixture(t)
		m, _ = m.Update(eventKey("home"))
		openedAt := m.clock.Add(20 * time.Second)
		m.now = func() time.Time { return openedAt }
		m, _ = m.Update(eventKey("n"))
		m.form.fields[0].SetValue("new activity")
		m.now = func() time.Time { return openedAt.Add(10 * time.Second) }
		m, cmd := m.Update(eventKey("enter"))
		saved := cmd().(Saved)
		saved.item.ActivityType = &s.label
		// Supply the persisted list returned after the existing Create call.
		s.items[0].EndedAt = &saved.item.StartedAt
		s.items = append(s.items, *saved.item)
		m, cmd = m.Update(saved)
		m = execute(t, m, cmd)
		view := ansi.Strip(m.View())
		box, now := strings.Index(view, "new activity"), strings.Index(view, "Now ·")
		if box < 0 || now < box || !m.following || m.items[m.index].EventID != saved.item.EventID || !m.clock.Equal(m.now()) {
			t.Fatalf("got selection %d and view:\n%s\nwant new event selected and visible above Now", m.items[m.index].EventID, view)
		}
	})
	t.Run("friendly input is converted before calling the existing service", func(t *testing.T) {
		m, s, _ := fixture(t)
		m, _ = m.Update(eventKey("n"))
		if got := m.form.fields[1].Value(); got != "2026-09-11 11:37:00 AM" {
			t.Fatalf("got default time %q, want full date and AM/PM", got)
		}
		m.form.fields[1].SetValue("2026-11-01 01:30:00 AM")
		m, cmd := m.Update(eventKey("enter"))
		if cmd != nil || s.creates != 0 || m.form.fields[1].Value() != "2026-11-01 01:30:00 AM" || !strings.Contains(m.form.message, "PDT or PST") {
			t.Fatal("ambiguous input should preserve draft and request a zone without saving")
		}
		m.form.fields[1].SetValue("2026-09-11 10:47:00 AM")
		m, cmd = m.Update(eventKey("enter"))
		cmd()
		want := time.Date(2026, 9, 11, 17, 47, 0, 0, time.UTC)
		if s.creates != 1 || !s.start.Equal(want) || s.start.Location() != time.UTC {
			t.Fatalf("got service time %v, want UTC %v", s.start, want)
		}
	})
	t.Run("End writes only after saving and retains version context", func(t *testing.T) {
		m, s, _ := fixture(t)
		m, _ = m.Update(eventKey("e"))
		if !m.form.ending || s.ends != 0 {
			t.Fatal("End form wrote before save")
		}
		m, cmd := m.Update(eventKey("enter"))
		result := cmd().(Saved)
		if s.ends != 1 || !result.ending || result.item.Version != 2 {
			t.Fatal("End lost version or called wrong operation")
		}
		_, quit := m.Update(tea.KeyPressMsg(tea.Key{Code: 'c', Mod: tea.ModCtrl}))
		if quit == nil {
			t.Fatal("Ctrl+C blocked by pending save")
		}
	})
	t.Run("opening and canceling do not write and Save uses captured time", func(t *testing.T) {
		m, s, _ := fixture(t)
		at := m.now()
		m, _ = m.Update(eventKey("n"))
		if !m.form.open || s.creates != 0 {
			t.Fatal("opening wrote or did not open")
		}
		m, _ = m.Update(eventKey("q"))
		if m.form.fields[0].Value() != "q" {
			t.Fatal("q not text")
		}
		m.now = func() time.Time { return at.Add(5 * time.Minute) }
		m, cmd := m.Update(eventKey("enter"))
		if !m.form.saving {
			t.Fatal("Save did not become pending")
		}
		_, duplicate := m.Update(eventKey("enter"))
		if duplicate != nil {
			t.Fatal("duplicate submission")
		}
		result := cmd().(Saved)
		if s.creates != 1 || !s.start.Equal(at) || s.label != "q" || result.err != nil {
			t.Fatal("Save lost original timestamp")
		}
		m, _ = m.Update(result)
		m, _ = m.Update(eventKey("n"))
		m, _ = m.Update(eventKey("esc"))
		if m.form.open || s.creates != 1 {
			t.Fatal("cancel wrote")
		}
	})
	t.Run("unsupported paste preserves draft and failure preserves original error", func(t *testing.T) {
		m, s, _ := fixture(t)
		m, _ = m.Update(eventKey("n"))
		m, _ = m.Update(tea.PasteMsg{Content: "draft"})
		m, _ = m.Update(tea.PasteMsg{Content: "bad\ninput"})
		if m.form.fields[0].Value() != "draft" || !strings.Contains(m.View(), "Input rejected") {
			t.Fatal("paste changed draft silently")
		}
		s.err = errors.New("PRIVATE_SAVE_MARKER")
		m, cmd := m.Update(eventKey("enter"))
		m, _ = m.Update(cmd())
		if m.form.err != s.err || m.form.fields[0].Value() != "draft" || strings.Contains(m.View(), "PRIVATE_SAVE_MARKER") {
			t.Fatal("failure lost draft or leaked error")
		}
	})
}
