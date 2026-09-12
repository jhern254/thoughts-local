package thoughts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/logging"
)

type totalSpy struct {
	Metrics
	calls int
	err   error
}

func (s *totalSpy) CountThoughts(ctx context.Context, userID string) (int64, error) {
	s.calls++
	if s.err != nil {
		return 0, s.err
	}
	return s.Metrics.CountThoughts(ctx, userID)
}

func TestModel_BrowseThoughtsViewTotal(t *testing.T) {
	t.Run("keeps the collection total while scrolling older and newer through a bounded window", func(t *testing.T) {
		m := browseThoughtsModel(t, 230)
		spy := &totalSpy{Metrics: m.browseThoughts.metrics}
		cmd := m.OpenBrowseThoughtsView(spy)
		batch := cmd().(tea.BatchMsg)
		// Browsing remains usable before the independent count arrives.
		m = runBrowseThoughtsCommand(m, batch[0])
		if len(m.browseThoughts.rows) != 51 || m.loading || !strings.Contains(m.View(), "Counting thoughts…") {
			t.Fatal("initial page did not become usable independently of the count")
		}
		count := batch[1]()
		for range 60 {
			m = browseThoughtsKey(m, tea.KeyDown)
		}
		m, _ = m.Update(count) // Cursor requests must not invalidate the count.
		if m.browseThoughts.countPending || m.browseThoughts.total != 230 {
			t.Fatal("scrolling invalidated count reply")
		}
		m = browseThoughtsKey(m, tea.KeyHome)
		if len(m.browseThoughts.rows) != 51 || !strings.Contains(m.View(), "\n230 thoughts\n") {
			t.Fatal("first 50 summaries did not display the full total")
		}
		for _, key := range []rune{tea.KeyDown, tea.KeyUp} {
			calls := spy.calls
			for range 220 {
				m = browseThoughtsKey(m, key)
			}
			if spy.calls != calls || !strings.Contains(m.View(), "\n230 thoughts\n") {
				t.Fatalf("scroll count calls: got %d, want %d", spy.calls, calls)
			}
		}
	})
	t.Run("creation refreshes the total when returning to the collection", func(t *testing.T) {
		m := browseThoughtsModel(t, 1)
		m = browseThoughtsKey(m, tea.KeyEnter)
		m, _ = m.Update(tea.PasteMsg{Content: "new thought"})
		m, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
		m = runBrowseThoughtsCommand(m, cmd)
		m = browseThoughtsKey(m, tea.KeyEscape)
		if !strings.Contains(m.View(), "\n2 thoughts\n") {
			t.Fatalf("got %q, want refreshed collection with 2 thoughts", m.View())
		}
	})
	t.Run("count failure preserves browsing and the original error without private logging", func(t *testing.T) {
		m := browseThoughtsModel(t, 2)
		var logs bytes.Buffer
		logger, err := logging.New(&logs, "test", "info")
		if err != nil {
			t.Fatal(err)
		}
		m.logger = logger
		private := errors.New("PRIVATE-TOTAL-MARKER")
		spy := &totalSpy{Metrics: m.browseThoughts.metrics, err: private}
		cmd := m.OpenBrowseThoughtsView(spy)
		batch := cmd().(tea.BatchMsg)
		m = runBrowseThoughtsCommand(m, batch[1]) // Count may fail before the page finishes.
		m = runBrowseThoughtsCommand(m, batch[0])
		if m.browseThoughts.countErr != private || m.err != nil || m.loading || !strings.Contains(m.View(), "Thought count unavailable") || strings.Contains(m.View(), "\n2 thoughts\n") {
			t.Fatalf("count failure replaced browsing or invented a total: %q", m.View())
		}
		var event map[string]any
		if err := json.Unmarshal(logs.Bytes(), &event); err != nil {
			t.Fatal(err)
		}
		if event["operation"] != "thought_count_all" || event["level"] != "error" {
			t.Fatalf("got event %v, want thought_count_all failure", event)
		}
		if strings.Contains(logs.String()+m.View(), "PRIVATE-TOTAL-MARKER") {
			t.Fatal("private error escaped")
		}
		m = browseThoughtsKey(m, tea.KeyDown)
		m = browseThoughtsKey(m, tea.KeyEnter)
		if !m.ShowingDetail() {
			t.Fatal("count failure prevented opening a thought")
		}
		m = browseThoughtsKey(m, tea.KeyEscape)
		spy.err = nil
		m = browseThoughtsKey(m, 'r')
		if m.browseThoughts.countErr != nil || !strings.Contains(m.View(), "\n2 thoughts\n") {
			t.Fatal("count did not recover on refresh")
		}
	})
}
