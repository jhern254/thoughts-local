package events

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/tui/displaytime"
)

func TestModel_Layout(t *testing.T) {
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
			if strings.Contains(date, "03-08") && strings.Contains(view, "02:00") {
				t.Fatal("rendered nonexistent hour")
			}
			if strings.Contains(date, "11-01") && (!strings.Contains(view, "01:00 PDT -07:00") || !strings.Contains(view, "01:00 PST -08:00")) {
				t.Fatal("lost repeated-hour distinction")
			}
		}
	})
	t.Run("completed and cross-midnight cards keep counts without a collapsed preview", func(t *testing.T) {
		m, _, _ := fixture(t)
		item := m.items[0]
		end := item.StartedAt.Add(time.Hour)
		item.EndedAt = &end
		card := ansi.Strip(m.card(item, false))
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
