package thoughts

import (
	"context"
	"errors"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
)

func TestModel_ListCount(t *testing.T) {
	for _, tt := range []struct {
		name, query, want string
		records           int
		createVisible     bool
	}{
		{"empty", "", "0 thoughts", 0, true},
		{"one record", "", "1 thought", 1, true},
		{"two records", "", "2 thoughts", 2, true},
		{"matching records across pages without Create", "alpha", "2 thoughts", 2, false},
		{"only Create matches", "Create", "0 thoughts", 2, true},
		{"zero matches", "zzzz", "0 thoughts", 2, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := New(context.Background(), "u", thought.NewService(testutils.NewFakeThoughtStore()), logging.Nop())
			rows := []list.Item{row{kind: rowCreate}, row{kind: rowRecord, item: data.Thought{ThoughtID: 1, Thought: "alpha one"}}, row{kind: rowRecord, item: data.Thought{ThoughtID: 2, Thought: "alpha two"}}}
			m.list.SetItems(rows[:tt.records+1])
			m.Resize(80, 14)
			if tt.query != "" {
				m.list.FilterInput.SetVirtualCursor(false)
				m, _ = m.Update(key('/'))
				var cmd tea.Cmd
				m, cmd = m.Update(tea.PasteMsg{Content: tt.query})
				m, _ = m.Update(thoughtFilterReply(t, cmd))
			}
			if tt.query == "alpha" && m.list.Paginator.TotalPages < 2 {
				t.Fatal("want matching records spread across pages")
			}
			if got := m.View(); !strings.Contains(got, "\n"+tt.want+"\n") {
				t.Fatalf("got view %q, want count %q", got, tt.want)
			}
			if tt.createVisible {
				if tt.query != "" {
					m, _ = m.Update(key(tea.KeyEnter)) // Accept the filter before selecting Create.
				}
				m, _ = m.Update(key(tea.KeyEnter))
				if m.screen != create || !m.input.Focused() {
					t.Fatal("Create is not selectable")
				}
			}
		})
	}
	t.Run("count preserves loading and error feedback", func(t *testing.T) {
		m := New(context.Background(), "u", thought.NewService(testutils.NewFakeThoughtStore()), logging.Nop())
		m.loading = true
		if got := m.View(); !strings.Contains(got, "Loading…") || !strings.Contains(got, "\n0 thoughts\n") {
			t.Fatalf("got view %q, want loading feedback and record count", got)
		}
		if got := strings.Count(m.View(), "\n") + 1; got > 14 {
			t.Fatalf("got %d rendered lines, want at most 14", got)
		}
		m.loading = false
		m.err = errors.New("failed")
		m.errMessage = "Could not load thoughts."
		if got := m.View(); !strings.Contains(got, m.errMessage) || !strings.Contains(got, "\n0 thoughts\n") {
			t.Fatalf("got view %q, want error feedback and record count", got)
		}
	})
}
