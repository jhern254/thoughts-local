package events

import (
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
)

func TestModel_Layout(t *testing.T) {
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
		for _, want := range []string{"12:00 AM", "11:00 AM", "Now · 11:37 AM ──", "Started 10:37 AM"} {
			if !strings.Contains(view, want) {
				t.Fatalf("got timeline %q, want %q", view, want)
			}
		}
		if strings.Contains(view, "PDT") || strings.Contains(view, "PST") || strings.Contains(view, "-07:00") {
			t.Fatalf("unexpected offset or 24-hour label: %s", view)
		}
		m = execute(t, m, m.openEvent(1))
		if !strings.Contains(ansi.Strip(m.View()), "Started 10:37:00 AM PDT") {
			t.Fatal("expanded event lost precise Pacific timestamp")
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
		for _, date := range []string{"2026-03-08 12:00:00 -07:00", "2026-11-01 12:00:00 -08:00"} {
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
		if !strings.Contains(card, "10:37 AM–11:37 AM") {
			t.Fatalf("got completed interval %q, want compact 12-hour start and end", card)
		}
		if strings.Contains(card, "preview") || !strings.Contains(card, "230 thoughts") || !strings.Contains(card, "Reading · 1h") {
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
