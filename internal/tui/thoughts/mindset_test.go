package thoughts

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
)

type mindsetWriter struct {
	Service
	at    time.Time
	calls int
	err   error
}

func (s *mindsetWriter) SetMindset(ctx context.Context, u string, id int64, active bool, at time.Time) (bool, error) {
	s.at = at
	s.calls++
	if s.err != nil {
		return false, s.err
	}
	return s.Service.SetMindset(ctx, u, id, active, at)
}

func TestModel_Mindset(t *testing.T) {
	for _, surface := range []string{"subject list", "browse list", "detail"} {
		t.Run("marks from "+surface+" at key time", func(t *testing.T) {
			service := thought.NewService(testutils.NewFakeThoughtStore())
			item, err := service.Create(t.Context(), "u", "Read carefully", nil, time.Unix(1, 0))
			if err != nil {
				t.Fatal(err)
			}
			writer := &mindsetWriter{Service: service}
			m := New(t.Context(), "u", writer, logging.Nop())
			cmd := m.OpenUnassigned()
			if surface == "browse list" {
				cmd = m.OpenBrowseThoughtsView(nil)
			}
			// Use the ordinary unassigned list for data loading; bounded view setup is
			// explicit here so unrelated metrics commands do not enter this test.
			if surface == "browse list" {
				m.loading = false
				m.browseThoughts.rows = []summaryRow{{kind: rowRecord, item: data.ThoughtSummaryView{ThoughtID: item.ThoughtID}}}
			} else {
				m, _ = m.Update(cmd())
				m.list.Select(1)
			}
			if surface == "detail" {
				m, cmd = m.Update(key(tea.KeyEnter))
				m, _ = m.Update(cmd())
			}
			captured := time.Unix(100, 0)
			m.now = func() time.Time { return captured }
			m, cmd = m.Update(key('m'))
			if cmd == nil {
				t.Fatal("missing mindset write")
			}
			m.now = func() time.Time { return captured.Add(time.Minute) }
			_, duplicate := m.Update(key('m'))
			if duplicate != nil {
				t.Fatal("repeated m started a second write")
			}
			m, changed := m.Update(cmd())
			if writer.at != captured || writer.calls != 1 {
				t.Fatalf("write: got %v, %d calls, want %v, 1", writer.at, writer.calls, captured)
			}
			if changed == nil {
				t.Fatal("missing badge invalidation")
			}
			m, _ = m.Update(changed())
			got, err := service.Get(t.Context(), "u", item.ThoughtID)
			if err != nil || !got.IsMindset {
				t.Fatalf("marked thought: got %+v, %v", got, err)
			}
		})
	}
	t.Run("m remains editor input", func(t *testing.T) {
		m := New(t.Context(), "u", thought.NewService(testutils.NewFakeThoughtStore()), logging.Nop())
		m.screen = create
		m.input.Focus()
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'm', Text: "m"}))
		if got := m.input.Value(); got != "m" {
			t.Fatalf("input: got %q, want m", got)
		}
	})
}

func TestModel_MindsetFailure(t *testing.T) {
	var logs bytes.Buffer
	logger, err := logging.New(&logs, "test", "info")
	if err != nil {
		t.Fatal(err)
	}
	service := thought.NewService(testutils.NewFakeThoughtStore())
	item, err := service.Create(t.Context(), "u", "PRIVATE THOUGHT", nil, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("PRIVATE ERROR")
	writer := &mindsetWriter{Service: service, err: failure}
	m := New(t.Context(), "u", writer, logger)
	cmd := m.OpenDetail(data.ThoughtSummaryView{ThoughtID: item.ThoughtID})
	m, _ = m.Update(cmd())
	m, cmd = m.Update(key('m'))
	m, _ = m.Update(cmd())
	if m.selected.IsMindset || m.mindsetSaving || m.err != failure || !strings.Contains(m.View(), "retry") {
		t.Fatalf("failed write state: marked=%v saving=%v err=%v", m.selected.IsMindset, m.mindsetSaving, m.err)
	}
	if strings.Contains(logs.String(), "PRIVATE") || strings.Contains(m.errMessage, "PRIVATE") {
		t.Fatal("private data reached diagnostics")
	}
	writer.err = nil
	m, cmd = m.Update(key('m'))
	m, _ = m.Update(cmd())
	if !m.selected.IsMindset || m.err != nil {
		t.Fatal("retry did not mark the thought")
	}
	if !strings.Contains(logs.String(), `"thought_id":1`) || strings.Contains(logs.String(), `"subject_id"`) {
		t.Fatalf("mutation metadata: got %s", logs.String())
	}
}

func TestModel_DirectThoughtDetail(t *testing.T) {
	t.Run("loading and failed reads never expose an unrelated create row", func(t *testing.T) {
		m := New(t.Context(), "u", failingService{err: errors.New("PRIVATE LOAD ERROR")}, logging.Nop())
		cmd := m.OpenDetail(data.ThoughtSummaryView{ThoughtID: 7})
		if strings.Contains(m.View(), "Create thought") {
			t.Fatal("direct detail showed a create row while loading")
		}
		m, _ = m.Update(cmd())
		if strings.Contains(m.View(), "Create thought") || strings.Contains(m.View(), "PRIVATE") || !strings.Contains(m.View(), "Could not load") {
			t.Fatalf("failed detail: got %q", m.View())
		}
		m, next := m.Update(key(tea.KeyEnter))
		if next != nil || m.Creating() {
			t.Fatal("failed detail entered the editor")
		}
		_, retry := m.Update(key('r'))
		if retry == nil {
			t.Fatal("missing read retry")
		}
	})
}

func TestModel_MindsetFilteredRows(t *testing.T) {
	m := filteringThoughts(t)
	for i, item := range m.list.Items() {
		row := item.(row)
		if row.kind == rowRecord {
			row.item.IsMindset = true
			m.list.SetItem(i, row)
		}
	}
	m, cmd := m.Update(tea.PasteMsg{Content: "alpha"})
	m, _ = m.Update(thoughtFilterReply(t, cmd))
	m, _ = m.Update(key(tea.KeyEnter))
	text := ansi.Strip(m.View())
	if !strings.Contains(text, " M  alpha") || strings.Contains(text, "[48;") {
		t.Fatalf("filtered badge and text: got %q, want intact M badge and alpha", text)
	}
	m, cmd = m.Update(key('m'))
	m, cmd = m.Update(cmd())
	m, cmd = m.Update(cmd())
	m, _ = m.Update(thoughtFilterReply(t, cmd))
	if m.list.SelectedItem().(row).item.IsMindset {
		t.Fatal("filtered row retained its badge after unmarking")
	}
}
