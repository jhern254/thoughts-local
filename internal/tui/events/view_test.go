package events

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"

	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
)

func TestModel_Layout(t *testing.T) {
	t.Run("adjacent event borders align with their intervals and contain intervening hours", func(t *testing.T) {
		m, _, _ := fixture(t)
		m.day = m.day.AddDate(0, 0, -1)
		start := m.day.Add(21*time.Hour + 7*time.Minute)
		end := start.Add(time.Hour)
		nextEnd := end.Add(time.Hour)
		m.items = []data.Event{{EventID: 1, StartedAt: start, EndedAt: &end}, {EventID: 2, StartedAt: end, EndedAt: &nextEnd}}
		lines, positions, _ := m.layout()
		first, second := positions[1], positions[2]
		if !strings.HasPrefix(ansi.Strip(lines[second-1]), "10:07 PM  ╰") || !strings.HasPrefix(ansi.Strip(lines[second]), "10:07 PM  ╭") {
			t.Fatalf("got boundary rows %q, want adjacent end/start borders at 10:07 PM", lines[second-1:second+1])
		}
		hour := slices.IndexFunc(lines, func(line string) bool { return strings.HasPrefix(line, "10:00 PM") })
		if hour <= first || hour >= second-1 || !strings.Contains(ansi.Strip(lines[hour]), "│") {
			t.Fatalf("got hour row %d, want inside event rows %d..%d", hour, first, second-1)
		}
		m.items[1].StartedAt = end.Add(30 * time.Minute)
		lines, positions, _ = m.layout()
		bottom := slices.IndexFunc(lines, func(line string) bool { return strings.HasPrefix(ansi.Strip(line), "10:07 PM  ╰") })
		if positions[2] <= bottom+1 {
			t.Fatal("a real half-hour gap did not leave space between boxes")
		}
	})
	t.Run("short zero-duration and cross-day events retain their exact border labels", func(t *testing.T) {
		m, _, _ := fixture(t)
		for _, span := range []time.Duration{0, time.Minute, 3 * time.Hour} {
			start := m.day.Add(8 * time.Hour)
			end := start.Add(span)
			m.items = []data.Event{{EventID: 1, StartedAt: start, EndedAt: &end}}
			lines, positions, _ := m.layout()
			top := positions[1]
			bottom := top + len(strings.Split(m.card(m.items[0], true), "\n")) - 1
			if !strings.HasPrefix(ansi.Strip(lines[bottom]), displaytime.Format(end, "03:04 PM")+"  ╰") || bottom-top < 3 {
				t.Fatalf("got span %s rows %q, want readable card ending at its timestamp", span, lines[top:bottom+1])
			}
		}
		m.items[0].StartedAt = m.day.Add(-time.Hour)
		m.items[0].EndedAt = nil
		m.day = m.day.AddDate(0, 0, -1)
		lines, positions, now := m.layout()
		if now != -1 || !strings.HasPrefix(ansi.Strip(lines[len(lines)-1]), "12:00 AM  ╰") || positions[1] >= len(lines)-1 {
			t.Fatal("ongoing event on completed day did not end at the midnight boundary")
		}
	})
	t.Run("thought count precedes collapsed and expanded previews", func(t *testing.T) {
		m, _, _ := fixture(t)
		for _, expanded := range []bool{false, true} {
			if expanded {
				m = execute(t, m, m.openEvent(1))
			}
			card := ansi.Strip(m.card(m.items[0], true))
			if count, preview := strings.Index(card, "230 thoughts"), strings.Index(card, "preview"); count < 0 || preview < 0 || count > preview {
				t.Fatalf("got card %q, want thought count above previews", card)
			}
		}
	})
	t.Run("styled header combines duration and compact timestamp range", func(t *testing.T) {
		for _, tc := range []struct{ start, end, want string }{
			{"10:47 PM", "11:00 PM", "13m · 10:47 - 11:00 PM"},
			{"10:47 AM", "11:00 AM", "13m · 10:47 - 11:00 AM"},
			{"11:30 AM", "12:30 PM", "1h · 11:30 AM - 12:30 PM"},
		} {
			t.Run(tc.want, func(t *testing.T) {
				m, _, _ := fixture(t)
				start, err := time.Parse("2006-01-02 03:04 PM -07:00", "2026-09-11 "+tc.start+" -07:00")
				if err != nil {
					t.Fatal(err)
				}
				end, err := time.Parse("2006-01-02 03:04 PM -07:00", "2026-09-11 "+tc.end+" -07:00")
				if err != nil {
					t.Fatal(err)
				}
				item := m.items[0]
				item.StartedAt, item.EndedAt = start, &end
				card := m.card(item, true)
				if !strings.Contains(card, list.DefaultStyles(true).Title.Render("Reading")) || !strings.Contains(ansi.Strip(strings.Split(card, "\n")[1]), tc.want) {
					t.Fatalf("got card %q, want styled Reading and %q on first content line", card, tc.want)
				}
			})
		}
	})
	t.Run("event expands beside its time and pushes later hours down", func(t *testing.T) {
		m, _, _ := fixture(t)
		m.items[0].StartedAt = m.day.Add(10 * time.Hour)
		lines, positions, _ := m.layout()
		top := positions[1]
		if !strings.HasPrefix(ansi.Strip(lines[top]), "10:00 AM  ╭") {
			t.Fatalf("got event row %q, want time beside top border", lines[top])
		}
		before := slices.IndexFunc(lines, func(line string) bool { return strings.HasPrefix(line, "11:00 AM") })
		m = execute(t, m, m.openEvent(1))
		lines, positions, _ = m.layout()
		after := slices.IndexFunc(lines, func(line string) bool { return strings.HasPrefix(line, "11:00 AM") })
		if after <= before || !strings.HasPrefix(ansi.Strip(lines[positions[1]]), "10:00 AM  ╭") {
			t.Fatal("inline expansion lost time column or failed to push later hour down")
		}
		if !strings.Contains(ansi.Strip(m.View()), "10:00 AM  ╭") {
			t.Fatal("expanded viewport hides event time")
		}
	})
	t.Run("default viewport spaces hours and shows only recent hours", func(t *testing.T) {
		m, _, _ := fixture(t)
		m.items = nil
		m.anchor()
		view := ansi.Strip(m.View())
		if strings.Contains(view, "12:00 AM") || strings.Contains(view, "06:00 AM") || !strings.Contains(view, "11:00 AM") {
			t.Fatalf("got default view %q, want recent hours rather than full day", view)
		}
		lines, _, _ := m.layout()
		if !strings.HasPrefix(lines[0], "12:00 AM") || !strings.Contains(lines[1], "│") || strings.Contains(lines[1], "AM") {
			t.Fatal("earlier hours unavailable or hour spacing missing")
		}
		m, _ = m.Update(eventKey("home"))
		if !strings.Contains(m.View(), "12:00 AM") {
			t.Fatal("Home cannot reach beginning of empty day")
		}
	})
	t.Run("collapsed calendar has no timezone even on selected event", func(t *testing.T) {
		m, _, _ := fixture(t)
		lines, _, _ := m.layout()
		view := ansi.Strip(strings.Join(lines, "\n"))
		for _, want := range []string{"12:00 AM", "11:00 AM", "Now · 11:37 AM ──", "1h · Started at 10:37 AM · ongoing"} {
			if !strings.Contains(view, want) {
				t.Fatalf("got timeline %q, want %q", view, want)
			}
		}
		if strings.Contains(view, "PDT") || strings.Contains(view, "PST") || strings.Contains(view, "-07:00") {
			t.Fatalf("unexpected offset or 24-hour label: %s", view)
		}
		m = execute(t, m, m.openEvent(1))
		if !strings.Contains(ansi.Strip(m.View()), "Started at 10:37 AM PDT · ongoing") {
			t.Fatal("expanded event lost Pacific timestamp")
		}
	})
	t.Run("current day ends at Now and completed days retain every hour", func(t *testing.T) {
		m, _, _ := fixture(t)
		m.items = nil
		lines, _, now := m.layout()
		if now != len(lines)-1 || strings.TrimSpace(lines[now]) != "── Now · 11:37 AM ──" {
			t.Fatalf("got current rail %q, want midnight through 11 AM ending at Now", lines)
		}
		m.clock = m.day.Add(12 * time.Hour)
		lines, _, now = m.layout()
		if now != len(lines)-1 || !strings.HasPrefix(lines[now-1], "12:00 PM") {
			t.Fatalf("got noon rail %q, want noon hour followed by Now", lines)
		}
		m.day = m.day.AddDate(0, 0, -1)
		lines, _, now = m.layout()
		if now != -1 || !strings.HasPrefix(lines[len(lines)-1], "11:00 PM") {
			t.Fatalf("got past rail %q, want 24 hours without Now", lines)
		}
		m.day = m.day.AddDate(0, 0, 2)
		lines, _, now = m.layout()
		if len(lines) != 0 || now != -1 {
			t.Fatalf("got future rail %q, want no elapsed hours", lines)
		}
	})
	for _, name := range []string{"collapsed", "expanded", "narrow", "empty"} {
		t.Run(name, func(t *testing.T) {
			m, _, _ := fixture(t)
			if name == "expanded" || name == "narrow" {
				var cmd = m.openEvent(1)
				m = execute(t, m, cmd)
			}
			if name == "narrow" {
				m.Resize(40, 20)
			}
			if name == "empty" {
				m.items = nil
				m.following = false
				m.offset = 0
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
	t.Run("hour rail handles missing and repeated hours", func(t *testing.T) {
		m, _, _ := fixture(t)
		m.items = nil
		for _, date := range []string{"2026-03-08 12:00:00 PM", "2026-11-01 12:00:00 PM"} {
			at, err := displaytime.ParseInput(date)
			if err != nil {
				t.Fatal(err)
			}
			m.clock = at
			m.day = displaytime.Day(at)
			lines, _, _ := m.layout()
			view := strings.Join(lines, "\n")
			if strings.Contains(date, "03-08") && strings.Contains(view, "02:00 AM") {
				t.Fatal("rendered nonexistent hour")
			}
			if strings.Contains(date, "11-01") && strings.Count(view, "01:00 AM") != 2 {
				t.Fatal("lost repeated hour")
			}
		}
	})
	t.Run("completed and cross-midnight cards keep counts without a collapsed preview", func(t *testing.T) {
		m, _, _ := fixture(t)
		item := m.items[0]
		end := item.StartedAt.Add(time.Hour)
		item.EndedAt = &end
		card := ansi.Strip(m.card(item, false))
		if !strings.Contains(card, "10:37 - 11:37 AM") {
			t.Fatalf("got completed interval %q, want compact 12-hour start and end", card)
		}
		if strings.Contains(card, "preview") || !strings.Contains(card, "230 thoughts") || !strings.Contains(card, " · 1h · ") {
			t.Fatal("completed summary did not retain duration/count only")
		}
		item.StartedAt = m.day.Add(-time.Hour)
		card = ansi.Strip(m.card(item, false))
		if !strings.Contains(card, "←") || !strings.Contains(card, "230 thoughts") {
			t.Fatal("cross-day card lost whole-event count or continuation")
		}
	})
	t.Run("midnight loads the next day once only while following", func(t *testing.T) {
		m, _, _ := fixture(t)
		oldDay := m.day
		m, cmd := m.Update(Tick{m.owner, m.session, m.day.AddDate(0, 0, 1).Add(time.Minute)})
		if m.day.Equal(oldDay) || cmd == nil || !m.loading {
			t.Fatal("following did not roll to next day")
		}
		m.following = false
		day := m.day
		m, _ = m.Update(Tick{m.owner, m.session, m.day.AddDate(0, 0, 1)})
		if !m.day.Equal(day) {
			t.Fatal("paused timeline changed date")
		}
	})
}
