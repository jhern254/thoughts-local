package thoughts

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
)

func TestModel_QualifiedThoughtDetails(t *testing.T) {
	t.Run("reload rechecks membership and Reset cancels reads without retaining the loader", func(t *testing.T) {
		service := thought.NewService(testutils.NewFakeThoughtStore())
		stored, err := service.Create(t.Context(), "u", "Normal service body", nil, time.Unix(100, 0))
		if err != nil {
			t.Fatal(err)
		}
		m := New(t.Context(), "u", service, logging.Nop())
		var calls int
		var readContext context.Context
		var failure error
		loader := func(ctx context.Context) (*data.Thought, error) {
			calls++
			readContext = ctx
			return &data.Thought{ThoughtID: stored.ThoughtID, Thought: "Qualified body"}, failure
		}
		cmd := m.OpenDetailWithLoader(data.ThoughtSummaryView{ThoughtID: stored.ThoughtID}, loader)
		m, _ = m.Update(cmd())
		m.Resize(48, 18)
		if !strings.Contains(m.View(), "m: mindset") {
			t.Fatal("narrow Details hides Mindset action")
		}
		if !strings.Contains(m.View(), "Qualified body") {
			t.Fatalf("detail: %s", m.View())
		}
		failure = data.ErrRecordNotFound
		m, cmd = m.Update(key('r'))
		m, _ = m.Update(cmd())
		if calls != 2 || m.selected != nil || strings.Contains(m.View(), "Qualified body") {
			t.Fatalf("failed scoped reload retained old Thought: calls %d, selected %+v", calls, m.selected)
		}
		m, cmd = m.Update(key('r'))
		late := cmd()
		m.Reset()
		m, _ = m.Update(late)
		if !errors.Is(readContext.Err(), context.Canceled) || m.selected != nil || m.detailLoader != nil {
			t.Fatal("Reset retained qualified ownership")
		}
		cmd = m.OpenDetail(data.ThoughtSummaryView{ThoughtID: stored.ThoughtID})
		m, _ = m.Update(cmd())
		if !strings.Contains(m.View(), "Normal service body") {
			t.Fatal("ordinary Details retained old qualified loader")
		}
	})
}
