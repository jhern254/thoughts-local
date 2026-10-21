package events

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
)

func TestModel_EventSubjectBadge(t *testing.T) {
	t.Run("places the assigned subject below the heading with a blank row before thoughts", func(t *testing.T) {
		m, events, _ := fixture(t)
		id := int64(7)
		events.items[0].SubjectID = &id
		reader := &subjectReaderStub{items: []data.Subject{{SubjectID: id, SubjectName: "Literature"}}}
		m.subjectReader = reader
		before := m.card(*m.entries[0].Event, true)
		if strings.Contains(before, "Literature") {
			t.Fatal("collapsed card displays expanded subject metadata")
		}
		next, cmd := m.Update(eventKey("enter"))
		m = execute(t, next, cmd)
		content := m.cardContent(*m.entries[0].Event)
		if got, want := content[1], list.DefaultStyles(true).Title.Render("Literature"); got != want {
			t.Fatalf("subject badge: got %q, want %q", got, want)
		}
		if content[2] != "" || !strings.Contains(content[3], "230 thoughts") {
			t.Fatalf("got card rows %q, want blank row between badge and thought count", content[:4])
		}
		for _, size := range [][2]int{{80, 27}, {40, 20}, {60, 22}} {
			m.Resize(size[0], size[1])
			rows, _, _ := m.layout()
			for _, row := range rows {
				if ansi.StringWidth(row) > size[0] {
					t.Fatalf("row exceeds %d cells: %q", size[0], row)
				}
				if size[0] < distributionMinWidth {
					continue
				}
				lane := ansi.Cut(ansi.Strip(row), 12, 22)
				hasBar := hasDistribution(lane) && strings.TrimSpace(lane) != "⢸"
				if got, want := hasBar, strings.Contains(ansi.Strip(row), "preview "); got != want {
					t.Fatalf("thought bar alignment: got %t, want %t in %q", got, want, row)
				}
			}
		}
		reader.items[0].SubjectName = "Renamed literature"
		next, cmd = m.Update(eventKey("r"))
		m = execute(t, next, cmd)
		if !strings.Contains(m.View(), "Renamed literature") {
			t.Fatal("refresh did not resolve the current subject name")
		}
		m.Resize(80, 27)
		m, _ = m.Update(eventKey("left"))
		if got := m.card(*m.entries[0].Event, true); got != before {
			t.Fatal("collapse changed the original compact card")
		}
	})

	t.Run("unassigned events keep their spacing without looking up a subject", func(t *testing.T) {
		m, _, _ := fixture(t)
		m.subjectReader = &subjectReaderStub{get: func(context.Context, string, int64) (*data.Subject, error) {
			t.Fatal("subject lookup for an unassigned event")
			return nil, nil
		}}
		m = execute(t, m, m.openEvent(1))
		content := m.cardContent(*m.entries[0].Event)
		if content[1] != "" || !strings.Contains(content[2], "230 thoughts") {
			t.Fatalf("got rows %q, want original unassigned spacing", content[:3])
		}
	})

	t.Run("clips long Unicode subject names without wrapping the badge", func(t *testing.T) {
		m, events, _ := fixture(t)
		id := int64(7)
		events.items[0].SubjectID = &id
		m.subjectReader = &subjectReaderStub{items: []data.Subject{{SubjectID: id, SubjectName: strings.Repeat("界", 70)}}}
		m = execute(t, m, m.openEvent(1))
		m.Resize(40, 20)
		content := m.cardContent(*m.entries[0].Event)
		if !strings.Contains(content[1], "界") || !strings.Contains(content[1], "…") || ansi.StringWidth(content[1]) > m.cardWidth() || content[2] != "" {
			t.Fatalf("got badge rows %q, want one clipped Unicode badge and a blank row", content[:3])
		}
	})

	t.Run("subject lookup failures leave thoughts available and report safe feedback", func(t *testing.T) {
		m, events, _ := fixture(t)
		id := int64(7)
		events.items[0].SubjectID = &id
		m.subjectReader = &subjectReaderStub{err: errors.New("PRIVATE_SUBJECT_ERROR")}
		var logs bytes.Buffer
		logger, err := logging.New(&logs, "test", "info")
		if err != nil {
			t.Fatal(err)
		}
		m.logger = logger
		m = execute(t, m, m.openEvent(1))
		view := ansi.Strip(m.View())
		if !strings.Contains(view, "Subject unavailable") || !strings.Contains(view, "preview 229") || strings.Contains(view+logs.String(), "PRIVATE_SUBJECT_ERROR") {
			t.Fatalf("got view %q and logs %q, want safe subject failure and usable thoughts", view, logs.String())
		}
		if strings.Count(logs.String(), "\n") != 1 || !strings.Contains(logs.String(), `"operation":"subject_get"`) {
			t.Fatalf("got logs %q, want one classified subject lookup failure", logs.String())
		}
	})

	t.Run("late subject replies cannot restore a collapsed card", func(t *testing.T) {
		m, events, _ := fixture(t)
		id := int64(7)
		events.items[0].SubjectID = &id
		var lookupCtx context.Context
		m.subjectReader = &subjectReaderStub{get: func(ctx context.Context, userID string, subjectID int64) (*data.Subject, error) {
			lookupCtx = ctx
			if userID != "u" || subjectID != id {
				t.Fatalf("got lookup (%q, %d), want (u, %d)", userID, subjectID, id)
			}
			return &data.Subject{SubjectID: id, SubjectName: "Old subject"}, nil
		}}
		m, cmd := m.Update(eventKey("enter"))
		reply := cmd()
		m, _ = m.Update(eventKey("left"))
		before := m.View()
		m, cmd = m.Update(reply)
		if lookupCtx == nil || !errors.Is(lookupCtx.Err(), context.Canceled) {
			t.Fatal("collapse did not cancel the subject lookup's owning context")
		}
		if cmd != nil || m.View() != before {
			t.Fatal("late subject reply changed the collapsed view")
		}
	})
}
