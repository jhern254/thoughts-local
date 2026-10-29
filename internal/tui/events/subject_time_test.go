package events

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"errors"
	"fmt"
	"math/bits"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/render"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
)

func TestModel_SubjectTimeNavigation(t *testing.T) {
	t.Run("short Now view keeps the latest card and boundary visible", func(t *testing.T) {
		m, _, _ := fixture(t)
		for _, height := range []int{10, 11, 12} {
			m.Resize(98, height)
			m, _ = m.Update(eventKey("end"))
			rows := strings.Split(ansi.Strip(m.timelineBody()), "\n")
			body := strings.Join(rows, "\n")
			if len(rows) != height-4 || !strings.Contains(body, "Now ·") || !strings.Contains(body, "Reading") {
				t.Fatalf("height %d: body=%q, want latest card and Now within %d rows", height, body, height-4)
			}
			if !strings.Contains(rows[0], "10:37 AM") || !strings.Contains(rows[0], "╭") {
				t.Fatalf("height %d: first row=%q, want latest card anchored at its start", height, rows[0])
			}
		}
	})

	t.Run("starts at Now then opens pinned metrics without reading", func(t *testing.T) {
		m, _, store := fixture(t)
		startup := New(m.ctx, m.userID, m.service, m.timelineView, thought.NewService(testutils.NewFakeThoughtStore()), m.subjectReader, m.logger)
		if startup.position.target != targetDayBoundary {
			t.Fatal("new model does not focus Now")
		}
		m, _ = m.Update(eventKey("end"))
		m.subjectTime.items = []data.SubjectTimeView{{SubjectID: 1, SubjectName: "Coding", TimeSpentSeconds: 12000}, {SubjectID: 2, SubjectName: "Reading", TimeSpentSeconds: 6000}}
		if m.position.target != targetDayBoundary {
			t.Fatal("startup did not focus Now")
		}
		rows := strings.Split(ansi.Strip(m.timelineBody()), "\n")
		if !strings.HasPrefix(rows[0], "10:37 AM") || !strings.Contains(rows[6], "Now") || !strings.Contains(rows[10], "Subject time today") || !strings.Contains(rows[11], "Coding") || !strings.Contains(rows[12], "Reading") || !strings.Contains(rows[13], "╰") {
			t.Fatalf("got Now stop %q, want latest event at top, Now below, and the full chart fitting beneath it", rows)
		}
		reads := store.subjectReads
		m, cmd := m.Update(eventKey("down"))
		if cmd != nil || store.subjectReads != reads || m.position.target != targetSubjectTime {
			t.Fatal("opening metrics started a read or lost focus")
		}
		rows = strings.Split(ansi.Strip(m.timelineBody()), "\n")
		if !strings.Contains(rows[0], "Now") || rows[1] != "" || rows[2] != "" || !strings.Contains(rows[3], "╭") || !strings.Contains(rows[4], "Subject time today") {
			t.Fatalf("pinned header: got %q", rows[:5])
		}
		m, _ = m.Update(eventKey("up"))
		if m.position.target != targetDayBoundary {
			t.Fatal("collapse skipped the Now stop")
		}
		m, _ = m.Update(eventKey("up"))
		if m.position.target != targetEntry {
			t.Fatal("Up did not select latest entry")
		}
		m, _ = m.Update(eventKey("down"))
		if m.position.target != targetDayBoundary {
			t.Fatal("Down skipped the buffer stop")
		}
	})
	t.Run("Now stop keeps only the latest event focused at the top and the boundary unstyled", func(t *testing.T) {
		m, _, _ := fixture(t)
		end := m.entries[0].StartedAt()
		label := "Earlier event"
		m.entries = append(eventEntries([]data.Event{{EventID: 2, ActivityType: &label, StartedAt: end.Add(-time.Hour), EndedAt: &end}}), m.entries...)
		m, _ = m.Update(eventKey("end"))
		rows := strings.Split(ansi.Strip(m.timelineBody()), "\n")
		if !strings.HasPrefix(rows[0], "10:37 AM") || !strings.Contains(rows[6], "Now") || strings.Contains(strings.Join(rows, "\n"), label) {
			t.Fatalf("got body %q, want only the latest card at the top followed by Now", rows)
		}
		layout := m.measureTimeline()
		if !layout.cards[len(layout.cards)-1].selected {
			t.Fatal("Now stop removed focus from the latest event")
		}
		want := strings.Repeat(" ", 24) + "── Now · 11:37 AM ──"
		if got := m.boundaryLine(); got != want {
			t.Fatalf("got boundary %q, want plain %q", got, want)
		}
		m, _ = m.Update(eventKey("down"))
		layout = m.measureTimeline()
		if layout.cards[len(layout.cards)-1].selected {
			t.Fatal("metrics retained event focus")
		}
	})

	t.Run("Up returns from metrics without another read", func(t *testing.T) {
		m, _, store := fixture(t)
		m, _ = m.Update(eventKey("end"))
		m, _ = m.Update(eventKey("down"))
		reads := store.subjectReads
		var cmd tea.Cmd
		m, cmd = m.Update(eventKey("up"))
		if cmd != nil || m.position.target != targetDayBoundary || store.subjectReads != reads {
			t.Fatal("Up did not return to the latest event and Now without a read")
		}
	})

	t.Run("End retains historical metrics until today's cards arrive", func(t *testing.T) {
		m, _, _ := fixture(t)
		m.day = m.day.AddDate(0, 0, -1)
		m.position.target = targetSubjectTime
		before := m.timelineBody()
		m, cmd := m.Update(eventKey("end"))
		if !m.load.entriesPending || m.load.retainedBody != before {
			t.Fatal("End changed the accepted historical body before today's reply")
		}
		m = execute(t, m, cmd)
		if m.position.target != targetDayBoundary || !m.day.Equal(displaytime.Day(m.clock)) {
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
		subjectID := int64(7)
		m.entries[0].Event.SubjectID = &subjectID
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
	t.Run("reads on ticks only when assigned ongoing time changes within the displayed day", func(t *testing.T) {
		for _, scenario := range []struct {
			name                                 string
			assigned, completed, empty           bool
			dayOffset                            int
			clockOffset, tickOffset, startOffset time.Duration
			wantRead                             bool
		}{
			{name: "empty day", empty: true, tickOffset: time.Minute},
			{name: "unassigned ongoing event", tickOffset: time.Minute},
			{name: "ongoing event has not started", assigned: true, startOffset: time.Hour, tickOffset: time.Minute},
			{name: "ongoing event starts during tick", assigned: true, startOffset: 30 * time.Second, tickOffset: time.Minute, wantRead: true},
			{name: "completed assigned event", assigned: true, completed: true, tickOffset: time.Minute},
			{name: "historical day with an ongoing assigned event", assigned: true, dayOffset: -1, tickOffset: time.Minute},
			{name: "future day", assigned: true, dayOffset: 1, tickOffset: time.Minute},
			{name: "unchanged second", assigned: true, tickOffset: time.Millisecond},
			{name: "assigned ongoing event", assigned: true, tickOffset: time.Minute, wantRead: true},
			{name: "final cutoff after midnight", assigned: true, clockOffset: 24*time.Hour - time.Second, tickOffset: time.Minute, wantRead: true},
			{name: "already clipped at midnight", assigned: true, clockOffset: 24 * time.Hour, tickOffset: time.Minute},
		} {
			t.Run(scenario.name, func(t *testing.T) {
				m, _, store := fixture(t)
				m.position.followNow = false
				if scenario.clockOffset != 0 {
					m.clock = m.day.Add(scenario.clockOffset)
				}
				m.day = m.day.AddDate(0, 0, scenario.dayOffset)
				event := m.entries[0].Event
				if scenario.startOffset != 0 {
					event.StartedAt = m.clock.Add(scenario.startOffset)
				}
				if scenario.empty {
					m.entries = nil
				}
				if scenario.assigned {
					id := int64(7)
					event.SubjectID = &id
				}
				if scenario.completed {
					end := m.clock
					event.EndedAt = &end
				}
				reads := store.subjectReads
				// End only the recurring clock; data commands keep the day context.
				clockCtx, cancelClock := context.WithCancel(t.Context())
				cancelClock()
				m.clockCtx = clockCtx
				m, cmd := m.Update(tickMsg{owner: m.owner, session: m.session, at: m.clock.Add(scenario.tickOffset)})
				m = execute(t, m, cmd)
				got := store.subjectReads - reads
				want := 0
				if scenario.wantRead {
					want = 1
				}
				if got != want {
					t.Fatalf("subject reads: got %d, want %d", got, want)
				}
			})
		}
	})

	t.Run("shows delayed feedback only for the current pending read", func(t *testing.T) {
		m, _, _ := fixture(t)
		m.position.target = targetSubjectTime
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
	t.Run("aligns Goals and durations three cells after the longest subject", func(t *testing.T) {
		m, _, _ := fixture(t)
		m.subjectTime.items = []data.SubjectTimeView{
			{SubjectID: 1, SubjectName: "Building something useful", TimeSpentSeconds: 12000},
			{SubjectID: 2, SubjectName: "Reading", TimeSpentSeconds: 6000},
		}
		rows := m.subjectTimePanel(false)
		heading, first, second := ansi.Strip(rows[1]), ansi.Strip(rows[2]), ansi.Strip(rows[3])
		if !strings.Contains(heading, "Subject time today") {
			t.Fatalf("got header %q, want title and Goals together", heading)
		}
		column := ansi.StringWidth(first[:strings.Index(first, "3h 20m")])
		for _, row := range []string{heading, second} {
			label := "Goals"
			if row == second {
				label = "1h 40m"
			}
			index := strings.Index(row, label)
			if index < 0 {
				t.Fatalf("missing %q in %q", label, row)
			}
			if got := ansi.StringWidth(row[:index]); got != column {
				t.Fatalf("column for %q: got %d, want %d", label, got, column)
			}
		}
		if !strings.Contains(first, "Building something useful    3h 20m") {
			t.Fatalf("got %q, want three spaces after the padded subject badge", first)
		}
	})

	t.Run("lays out metrics directly below Now independently of the selected event", func(t *testing.T) {
		m, _, _ := fixture(t)
		m.subjectTime.items = []data.SubjectTimeView{{SubjectID: 1, SubjectName: "Coding", TimeSpentSeconds: 3600}, {SubjectID: 2, SubjectName: "Reading", TimeSpentSeconds: 1800}}
		end := m.entries[0].StartedAt()
		m.entries = append(eventEntries([]data.Event{{EventID: 2, StartedAt: end.Add(-time.Hour), EndedAt: &end}}), m.entries...)
		m.position.entryIndex = 0
		m.position.followNow = false
		layout := m.measureTimeline()
		m.position.topLine = layout.nowLine - 8
		rows := strings.Split(ansi.Strip(m.timelineBody()), "\n")
		if !strings.Contains(rows[8], "Now") || strings.TrimSpace(rows[9]) != "" || strings.TrimSpace(rows[10]) != "" || !strings.Contains(rows[11], "╭") || !strings.Contains(rows[12], "Subject time today") || !strings.Contains(rows[13], "Coding") || !strings.Contains(rows[14], "Reading") || !strings.Contains(rows[15], "╰") {
			t.Fatalf("got rows %q, want the complete card immediately after Now and a two-row gap", rows)
		}
		m.position.entryIndex = 1
		if got := ansi.Strip(m.timelineBody()); got != strings.Join(rows, "\n") {
			t.Fatal("selection changed the metric card's position or visibility")
		}
		m.position.topLine = 0
		if strings.Contains(m.timelineBody(), "Subject time today") {
			t.Fatal("metrics remained pinned while browsing earlier timeline rows")
		}
	})

	t.Run("keeps durations and Unicode names inside a narrow card without a chart lane", func(t *testing.T) {
		m, _, _ := fixture(t)
		m.Resize(40, 20)
		m.position.target = targetSubjectTime
		m.subjectTime.items = []data.SubjectTimeView{{SubjectID: 7, SubjectName: strings.Repeat("学習", 30), TimeSpentSeconds: 5400}}
		rows := strings.Split(ansi.Strip(m.timelineBody()), "\n")
		if !strings.Contains(rows[4], "Subject time today") || !strings.Contains(rows[4], "Goals") {
			t.Fatalf("got narrow header %q, want both labels on one row", rows[4])
		}
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
		m.position.target = targetSubjectTime
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

	t.Run("anchors empty and short charts below Now and labels historical days", func(t *testing.T) {
		m, _, _ := fixture(t)
		m.entries = nil
		m.position.target = targetDayBoundary
		rows := strings.Split(ansi.Strip(m.timelineBody()), "\n")
		if len(rows) != m.bodyHeight() || !strings.Contains(rows[0], "Now") || !strings.Contains(rows[5], "No subject time recorded") || !strings.Contains(rows[6], "╰") {
			t.Fatalf("got empty preview %q", rows)
		}
		m.day = m.day.AddDate(0, 0, -1)
		m.position.target = targetSubjectTime
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
	m.position.target = targetDayBoundary
	reads := store.subjectReads
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		m, _ = m.Update(eventKey("down"))
		_ = m.View()
		m, _ = m.Update(eventKey("up"))
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
			m.position.target = targetDayBoundary
			m.subjectTime.items = []data.SubjectTimeView{
				{SubjectID: 1, SubjectName: "Coding", TimeSpentSeconds: 12000},
				{SubjectID: 2, SubjectName: "Reading", TimeSpentSeconds: 6000},
				{SubjectID: 3, SubjectName: "Planning", TimeSpentSeconds: 1200},
			}
			if name != "subject-time-preview" {
				m.position.target = targetSubjectTime
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

func TestModel_SubjectTimeDistributions(t *testing.T) {
	t.Run("uses a strong baseline and responds to shared controls from timeline and metrics", func(t *testing.T) {
		for _, target := range []timelineTarget{targetDayBoundary, targetSubjectTime} {
			m, _, _ := fixture(t)
			m.position.target = target
			m.subjectTime.items = []data.SubjectTimeView{
				{SubjectID: 1, SubjectName: "Coding", TimeSpentSeconds: 3600},
				{SubjectID: 2, SubjectName: "Reading", TimeSpentSeconds: 1800},
			}
			before := distributionLane(m.subjectTimePanel(false))
			// At normal shared settings the half-day bar has Strong Boost's
			// 0.9375 exponent, reaching eight dots plus its baseline: 36 dots.
			rows := m.subjectTimePanel(false)
			lane := ansi.Cut(ansi.Strip(rows[3]), 12, 22)
			var dots int
			for _, glyph := range lane {
				if glyph >= '\u2800' && glyph <= '\u28ff' {
					dots += bits.OnesCount8(uint8(glyph - '\u2800'))
				}
			}
			if dots != 36 {
				t.Fatalf("default metric bar: got %d dots, want 36", dots)
			}
			m, _ = m.Update(eventKey("d"))
			m, _ = m.Update(eventKey("left"))
			previous := 0
			for level := range boostCount {
				lane := distributionLane(m.subjectTimePanel(false))
				filled := 0
				for _, glyph := range lane {
					if glyph >= '\u2800' && glyph <= '\u28ff' {
						filled += bits.OnesCount8(uint8(glyph - '\u2800'))
					}
				}
				if level > 0 && filled <= previous {
					t.Fatalf("target %v boost %d: got %d dots, want more than %d", target, level, filled, previous)
				}
				previous = filled
				m, _ = m.Update(eventKey("right"))
			}
			m, _ = m.Update(eventKey("up"))
			m, _ = m.Update(eventKey("f"))
			if m.distributions.size != 105 || m.distributions.mode != render.Outline {
				t.Fatal("shared size and mode did not change")
			}
			m, _ = m.Update(eventKey("0"))
			if got := distributionLane(m.subjectTimePanel(false)); got != before {
				t.Fatal("reset did not restore strong metric geometry")
			}
			m, _ = m.Update(eventKey("esc"))
			if m.distributions.open {
				t.Fatal("tuning did not close")
			}
		}
	})
}

func TestModel_SubjectTimeDayNavigation(t *testing.T) {
	t.Run("cycles days while keeping metrics focused and stops at today", func(t *testing.T) {
		m, _, _ := fixture(t)
		m, _ = m.Update(eventKey("end"))
		m, _ = m.Update(eventKey("down"))
		today := m.day
		m, cmd := m.Update(eventKey("left"))
		if cmd == nil {
			t.Fatal("Left did not request the previous day")
		}
		if !m.day.Equal(today) || m.position.target != targetSubjectTime {
			t.Fatal("pending navigation lost the accepted day or metrics focus")
		}
		m = execute(t, m, cmd)
		if !m.day.Equal(today.AddDate(0, 0, -1)) || m.position.target != targetSubjectTime || !m.subjectTime.itemsDay.Equal(m.day) {
			t.Fatal("previous day did not retain metrics focus with matching data")
		}
		m, cmd = m.Update(eventKey("right"))
		m = execute(t, m, cmd)
		if !m.day.Equal(today) || m.position.target != targetSubjectTime {
			t.Fatal("Right did not return to today's metrics")
		}
		m, cmd = m.Update(eventKey("right"))
		if cmd != nil || m.position.target != targetSubjectTime || !m.expanded.IsZero() {
			t.Fatal("Right at today navigated to the future or opened an Event")
		}
	})
	t.Run("pending repeats accumulate and stale lists cannot steal metrics focus", func(t *testing.T) {
		m, _, _ := fixture(t)
		m.day = m.day.AddDate(0, 0, -5)
		m.position.target = targetSubjectTime
		start := m.day
		m, first := m.Update(eventKey("left"))
		if first == nil {
			t.Fatal("missing first read")
		}
		old := first().(tea.BatchMsg)[0]()
		m, _ = m.Update(eventKey("left"))
		m, last := m.Update(eventKey("right"))
		if last == nil {
			t.Fatal("missing final read")
		}
		m = execute(t, m, last)
		m = execute(t, m, func() tea.Msg { return old })
		if !m.day.Equal(start.AddDate(0, 0, -1)) || m.position.target != targetSubjectTime {
			t.Fatal("burst or stale reply changed the accepted destination/focus")
		}
	})
}
