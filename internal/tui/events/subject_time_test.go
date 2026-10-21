package events

import (
	tea "charm.land/bubbletea/v2"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
)

func TestModel_SubjectTimeNavigation(t *testing.T) {
	t.Run("starts at Now then opens pinned metrics without reading", func(t *testing.T) {
		m, _, store := fixture(t)
		startup := New(m.ctx, m.userID, m.service, m.timelineView, thought.NewService(testutils.NewFakeThoughtStore()), m.subjectReader, m.logger)
		if startup.position.target != targetBoundary {
			t.Fatal("new model does not focus Now")
		}
		m, _ = m.Update(eventKey("end"))
		m.subjectTime.items = []data.SubjectTimeView{{SubjectID: 1, SubjectName: "Coding", TimeSpentSeconds: 12000}, {SubjectID: 2, SubjectName: "Reading", TimeSpentSeconds: 6000}}
		if m.position.target != targetBoundary {
			t.Fatal("startup did not focus Now")
		}
		rows := strings.Split(ansi.Strip(m.timelineBody()), "\n")
		if !strings.Contains(rows[len(rows)-1], "Coding") || strings.Contains(rows[len(rows)-1], "╰") {
			t.Fatalf("bottom preview: got %q, want leading subject at bottom edge", rows)
		}
		if !strings.Contains(rows[len(rows)-6], "Now") {
			t.Fatalf("Now position: got %q, want five preview rows beneath Now", rows)
		}
		reads := store.subjectReads
		m, cmd := m.Update(eventKey("down"))
		if cmd != nil || store.subjectReads != reads || m.position.target != targetMetrics {
			t.Fatal("opening metrics started a read or lost focus")
		}
		rows = strings.Split(ansi.Strip(m.timelineBody()), "\n")
		if !strings.Contains(rows[0], "Now") || rows[1] != "" || rows[2] != "" || !strings.Contains(rows[3], "╭") || !strings.Contains(rows[4], "Subject time today") {
			t.Fatalf("pinned header: got %q", rows[:5])
		}
		m, _ = m.Update(eventKey("left"))
		if m.position.target != targetBoundary {
			t.Fatal("collapse skipped the Now stop")
		}
		m, _ = m.Update(eventKey("up"))
		if m.position.target != targetEntry {
			t.Fatal("Up did not select latest entry")
		}
		m, _ = m.Update(eventKey("down"))
		if m.position.target != targetBoundary {
			t.Fatal("Down skipped the buffer stop")
		}
	})
	t.Run("End retains historical metrics until today's cards arrive", func(t *testing.T) {
		m, _, _ := fixture(t)
		m.day = m.day.AddDate(0, 0, -1)
		m.position.target = targetMetrics
		before := m.timelineBody()
		m, cmd := m.Update(eventKey("end"))
		if !m.load.entriesPending || m.load.retainedBody != before {
			t.Fatal("End changed the accepted historical body before today's reply")
		}
		m = execute(t, m, cmd)
		if m.position.target != targetBoundary || !m.day.Equal(displaytime.Day(m.clock)) {
			t.Fatal("End did not land at today's Now stop")
		}
	})

	t.Run("scrolls subjects beneath pinned header and overflow markers", func(t *testing.T) {
		m, _, _ := fixture(t)
		m, _ = m.Update(eventKey("end"))
		for i := 0; i < 30; i++ {
			m.subjectTime.items = append(m.subjectTime.items, data.SubjectTimeView{SubjectID: int64(i + 1), SubjectName: fmt.Sprintf("Subject %02d", i), TimeSpentSeconds: int64(3600 - i)})
		}
		m, _ = m.Update(eventKey("down"))
		m, _ = m.Update(eventKey("down"))
		rows := strings.Split(ansi.Strip(m.timelineBody()), "\n")
		if !strings.Contains(rows[0], "Now") || !strings.Contains(rows[4], "Subject time today") || !strings.Contains(rows[5], "...") || !strings.Contains(rows[6], "Subject 01") {
			t.Fatalf("scrolled header: got %q", rows[:7])
		}
		if !strings.Contains(rows[len(rows)-2], "...") || !strings.Contains(rows[len(rows)-1], "╰") {
			t.Fatalf("bottom overflow: got %q", rows[len(rows)-2:])
		}
	})
}

func TestModel_SubjectTimeRefresh(t *testing.T) {
	t.Run("rejects older revisions and cancelled work without changing retained data", func(t *testing.T) {
		m, _, store := fixture(t)
		store.subjectTimes = []data.SubjectTimeView{{SubjectID: 1, SubjectName: "Old", TimeSpentSeconds: 60}}
		old := m.loadSubjectTime(m.day)().(tea.BatchMsg)
		reply := old[0]()
		store.subjectTimes = []data.SubjectTimeView{{SubjectID: 2, SubjectName: "New", TimeSpentSeconds: 120}}
		current := m.loadSubjectTime(m.day)
		m, _ = m.Update(reply)
		if len(m.subjectTime.items) != 0 || !m.subjectTime.pending {
			t.Fatal("obsolete revision replaced data or finished the new read")
		}
		if old[1]() != nil {
			t.Fatal("cancelled feedback produced a message")
		}
		m = execute(t, m, current)
		if len(m.subjectTime.items) != 1 || m.subjectTime.items[0].SubjectName != "New" {
			t.Fatalf("got %v, want latest revision", m.subjectTime.items)
		}
		cancelled := m.loadSubjectTime(m.day)().(tea.BatchMsg)
		reads := store.subjectReads
		m.Close()
		if cancelled[0]() != nil || cancelled[1]() != nil || store.subjectReads != reads {
			t.Fatal("closed model executed cancelled metric work")
		}
	})
	t.Run("waits for the matching day and rejects replies from an abandoned day", func(t *testing.T) {
		m, _, store := fixture(t)
		store.subjectTimes = []data.SubjectTimeView{{SubjectID: 3, SubjectName: "Yesterday", TimeSpentSeconds: 180}}
		day := m.day.AddDate(0, 0, -1)
		batch := m.loadDay(day)().(tea.BatchMsg)
		metric := batch[3]().(tea.BatchMsg)
		reply := metric[0]()
		m, _ = m.Update(reply)
		if len(m.subjectTime.items) != 0 || m.day.Equal(day) {
			t.Fatal("early metrics replaced the displayed day")
		}
		m, _ = m.Update(batch[0]())
		if !m.day.Equal(day) || len(m.subjectTime.items) != 1 || m.subjectTime.items[0].SubjectName != "Yesterday" {
			t.Fatal("matching day did not adopt its early metrics")
		}
		next := m.loadDay(day.AddDate(0, 0, -1))
		m, _ = m.Update(reply)
		if !m.subjectTime.pending {
			t.Fatal("abandoned day finished a newer read")
		}
		m = execute(t, m, next)
	})
	t.Run("refreshes ongoing time on a clock tick and retains same-day data on failure", func(t *testing.T) {
		m, _, store := fixture(t)
		store.subjectTimes = []data.SubjectTimeView{{SubjectID: 7, SubjectName: "Coding", TimeSpentSeconds: 300}}
		at := m.clock.Add(time.Minute)
		m, cmd := m.Update(tickMsg{owner: m.owner, session: m.session, at: at})
		m = execute(t, m, cmd().(tea.BatchMsg)[0])
		if store.subjectCutoff != at || len(m.subjectTime.items) != 1 {
			t.Fatalf("got cutoff %v, items %v; want tick cutoff %v", store.subjectCutoff, m.subjectTime.items, at)
		}
		store.subjectTimeErr = errors.New("PRIVATE_SUBJECT_TIME_FAILURE")
		m = execute(t, m, m.loadSubjectTime(m.day))
		if len(m.subjectTime.items) != 1 || m.subjectTime.err != store.subjectTimeErr || strings.Contains(m.View(), "PRIVATE_SUBJECT_TIME_FAILURE") {
			t.Fatal("refresh failure discarded good data or exposed raw error")
		}
	})
	t.Run("shows delayed feedback only for the current pending read", func(t *testing.T) {
		m, _, _ := fixture(t)
		m.position.target = targetMetrics
		batch := m.loadSubjectTime(m.day)().(tea.BatchMsg)
		if strings.Contains(m.View(), "Loading subject time") {
			t.Fatal("fast read flashed loading feedback")
		}
		m, _ = m.Update(subjectTimeDelayedMsg{m.owner, m.load.generation, m.subjectTime.revision - 1})
		if m.subjectTime.showStatus {
			t.Fatal("obsolete delay showed feedback")
		}
		m, _ = m.Update(subjectTimeDelayedMsg{m.owner, m.load.generation, m.subjectTime.revision})
		if !strings.Contains(m.View(), "Loading subject time") {
			t.Fatal("slow pending read did not show feedback")
		}
		m, _ = m.Update(batch[0]())
		if strings.Contains(m.View(), "Loading subject time") || batch[1]() != nil {
			t.Fatal("finished read retained feedback or its timer")
		}
	})

	t.Run("preserves the top subject when a refresh changes ranking", func(t *testing.T) {
		m, _, store := fixture(t)
		for i := 0; i < 30; i++ {
			store.subjectTimes = append(store.subjectTimes, data.SubjectTimeView{SubjectID: int64(i + 1), SubjectName: fmt.Sprintf("Subject %02d", i), TimeSpentSeconds: int64(3600 - i)})
		}
		m = execute(t, m, m.loadSubjectTime(m.day))
		m.subjectTime.offset = 4
		store.subjectTimes = append([]data.SubjectTimeView(nil), store.subjectTimes...)
		store.subjectTimes[4], store.subjectTimes[5] = store.subjectTimes[5], store.subjectTimes[4]
		m = execute(t, m, m.loadSubjectTime(m.day))
		if m.subjectTime.offset != 5 || m.subjectTime.items[5].SubjectID != 5 {
			t.Fatalf("got offset %d, want subject 5 retained at index 5", m.subjectTime.offset)
		}
	})
}

func TestModel_SubjectTimePresentation(t *testing.T) {
	t.Run("keeps durations and Unicode names inside a narrow card without a chart lane", func(t *testing.T) {
		m, _, _ := fixture(t)
		m.Resize(40, 20)
		m.position.target = targetMetrics
		m.subjectTime.items = []data.SubjectTimeView{{SubjectID: 7, SubjectName: strings.Repeat("学習", 30), TimeSpentSeconds: 5400}}
		rows := strings.Split(ansi.Strip(m.timelineBody()), "\n")
		if !strings.Contains(rows[5], "1h 30m") || !strings.Contains(rows[5], "…") {
			t.Fatalf("got %q, want truncated name and full duration", rows[5])
		}
		for _, row := range rows {
			if ansi.StringWidth(row) > 40 {
				t.Fatalf("row width %d exceeds 40: %q", ansi.StringWidth(row), row)
			}
			for _, glyph := range row {
				if glyph >= '\u2800' && glyph <= '\u28ff' {
					t.Fatalf("narrow chart contains Braille: %q", row)
				}
			}
		}
	})
	t.Run("uses the whole-day scale while scrolling and leaves ellipsis lanes blank", func(t *testing.T) {
		m, _, _ := fixture(t)
		m.position.target = targetMetrics
		for i := 0; i < 30; i++ {
			m.subjectTime.items = append(m.subjectTime.items, data.SubjectTimeView{SubjectID: int64(i + 1), SubjectName: fmt.Sprintf("Subject %02d", i), TimeSpentSeconds: int64(36000 / (i + 1))})
		}
		before := strings.Split(ansi.Strip(m.timelineBody()), "\n")
		m, _ = m.Update(eventKey("down"))
		after := strings.Split(ansi.Strip(m.timelineBody()), "\n")
		lane := func(row string) string { return ansi.Truncate(row, m.cardColumn(), "") }
		if lane(before[6]) != lane(after[6]) || lane(after[6]) == lane(before[5]) {
			t.Fatal("scrolling rescaled the second subject to the offscreen leader")
		}
		if strings.TrimSpace(lane(after[5])) != "" || strings.TrimSpace(lane(after[len(after)-2])) != "" {
			t.Fatal("overflow marker carries a chart bar")
		}
	})

	t.Run("pads above empty and short previews and labels historical days", func(t *testing.T) {
		m, _, _ := fixture(t)
		m.entries = nil
		m.position.target = targetBoundary
		rows := strings.Split(ansi.Strip(m.timelineBody()), "\n")
		if len(rows) != m.bodyHeight() || !strings.Contains(rows[len(rows)-1], "No subject time recorded") || !strings.Contains(rows[len(rows)-6], "Now") {
			t.Fatalf("got empty preview %q", rows)
		}
		m.day = m.day.AddDate(0, 0, -1)
		m.position.target = targetMetrics
		rows = strings.Split(ansi.Strip(m.timelineBody()), "\n")
		if strings.Contains(rows[0], "Now") || !strings.Contains(rows[0], "...") || strings.Contains(rows[4], "today") {
			t.Fatalf("got historical heading %q", rows[:5])
		}
	})
}

func BenchmarkModel_SubjectTimeNavigation(b *testing.B) {
	m, _, store := fixture(b)
	for i := 0; i < 30; i++ {
		m.subjectTime.items = append(m.subjectTime.items, data.SubjectTimeView{SubjectID: int64(i + 1), SubjectName: fmt.Sprintf("Subject %02d", i), TimeSpentSeconds: int64(3600 - i)})
	}
	m.position.target = targetBoundary
	reads := store.subjectReads
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		m, _ = m.Update(eventKey("down"))
		_ = m.View()
		m, _ = m.Update(eventKey("left"))
		_ = m.View()
	}
	b.StopTimer()
	if store.subjectReads != reads {
		b.Fatal("navigation read subject metrics")
	}
}

func TestModel_SubjectTimeSnapshots(t *testing.T) {
	for _, name := range []string{"subject-time-preview", "subject-time-expanded", "subject-time-narrow"} {
		t.Run(name, func(t *testing.T) {
			m, _, _ := fixture(t)
			m.position.target = targetBoundary
			m.subjectTime.items = []data.SubjectTimeView{
				{SubjectID: 1, SubjectName: "Coding", TimeSpentSeconds: 12000},
				{SubjectID: 2, SubjectName: "Reading", TimeSpentSeconds: 6000},
				{SubjectID: 3, SubjectName: "Planning", TimeSpentSeconds: 1200},
			}
			if name != "subject-time-preview" {
				m.position.target = targetMetrics
			}
			if name == "subject-time-narrow" {
				m.Resize(40, 20)
			}
			got := ansi.Strip(m.View()) + "\n"
			want, err := os.ReadFile("testdata/" + name + ".txt")
			if err != nil {
				t.Fatal(err)
			}
			if got != string(want) {
				t.Fatalf("got layout:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}
