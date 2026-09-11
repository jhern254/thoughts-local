package thoughts

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
)

func key(code rune) tea.KeyPressMsg { return tea.KeyPressMsg(tea.Key{Code: code}) }

func TestModel_Thoughts(t *testing.T) {
	t.Run("Create description follows the selected scope before and after loading", func(t *testing.T) {
		m := New(context.Background(), "u", thought.NewService(testutils.NewFakeThoughtStore()), logging.Nop())
		for _, misc := range []bool{true, false, true} {
			want := "Add a thought to this subject"
			var cmd tea.Cmd
			if misc {
				want = "Write a new thought"
				cmd = m.OpenUnassigned()
			} else {
				cmd = m.Open(7)
			}
			for phase := range 2 {
				if phase == 1 {
					m, _ = m.Update(cmd())
				}
				if got := m.list.Items()[0].(row).Description(); got != want {
					t.Fatalf("got Create description %q, want %q", got, want)
				}
			}
		}
	})
	t.Run("late unassigned creation keeps its scope without replacing the active subject", func(t *testing.T) {
		service := thought.NewService(testutils.NewFakeThoughtStore())
		m := New(context.Background(), "u", service, logging.Nop())
		m.OpenUnassigned()
		old := m.createThought("complete Misc draft")
		current := m.Open(7)
		result := old().(Result)
		if result.err != nil || result.item.SubjectID != nil || result.item.Thought != "complete Misc draft" {
			t.Fatalf("got result %+v, want original unassigned creation", result)
		}
		m, cmd := m.Update(result)
		if !m.loading || m.subjectID == nil || *m.subjectID != 7 || m.selected != nil || cmd != nil {
			t.Fatal("late Misc creation replaced active subject")
		}
		m, _ = m.Update(current())
		if m.loading || len(m.list.Items()) != 1 {
			t.Fatal("active subject list lost its empty state")
		}
		old = m.listThoughts()
		current = m.OpenUnassigned()
		m, _ = m.Update(current())
		m, _ = m.Update(old())
		if m.subjectID != nil || len(m.list.Items()) != 2 {
			t.Fatal("old subject list replaced Misc rows")
		}
	})
	for _, tt := range []struct {
		name  string
		row   row
		title string
		desc  string
	}{
		{"create action does not depend on thought ID", row{kind: rowCreate, item: data.Thought{ThoughtID: 7}}, "Create thought…", "Add a thought to this subject"},
		{"record with zero ID is not a create action", row{kind: rowRecord, item: data.Thought{Thought: "record", ObservedAt: time.Date(2026, time.January, 2, 15, 4, 0, 0, time.UTC)}}, "record", "Jan 2, 2026 7:04 AM PST"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.row.Title() != tt.title || tt.row.FilterValue() != tt.title || tt.row.Description() != tt.desc {
				t.Fatalf("got row %q / %q / %q, want %q / %q / %q", tt.row.Title(), tt.row.FilterValue(), tt.row.Description(), tt.title, tt.title, tt.desc)
			}
			m := New(context.Background(), "u", thought.NewService(testutils.NewFakeThoughtStore()), logging.Nop())
			m.list.SetItems([]list.Item{tt.row})
			m, cmd := m.Update(key(tea.KeyEnter))
			if tt.row.kind == rowCreate {
				if m.screen != create || !m.input.Focused() || m.loading {
					t.Fatal("create action did not open the editor")
				}
			} else {
				if m.screen != browse || !m.loading || cmd == nil {
					t.Fatal("record did not start loading its detail")
				}
				if result := cmd().(Result); result.operation != logging.ThoughtGet {
					t.Fatal("record did not request a thought")
				}
			}
		})
	}
	t.Run("editor treats Enter and subject shortcuts as input and Escape cancels", func(t *testing.T) {
		m := New(context.Background(), "u", thought.NewService(testutils.NewFakeThoughtStore()), logging.Nop())
		cmd := m.Open(1)
		m, _ = m.Update(cmd())
		m, _ = m.Update(key(tea.KeyEnter))
		for _, r := range "edq" {
			m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
		}
		m, _ = m.Update(key(tea.KeyEnter))
		if m.input.Value() != "edq\n" || m.Browsing() {
			t.Fatalf("got input %q, browsing %v", m.input.Value(), m.Browsing())
		}
		m, _ = m.Update(key(tea.KeyEscape))
		if !m.Browsing() || m.input.Value() != "" {
			t.Fatal("cancel did not return to clean list")
		}
	})
	t.Run("filtering owns navigation until Escape clears it", func(t *testing.T) {
		m := New(context.Background(), "u", thought.NewService(testutils.NewFakeThoughtStore()), logging.Nop())
		cmd := m.Open(1)
		m, _ = m.Update(cmd())
		m, _ = m.Update(key('/'))
		m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'q', Text: "q"}))
		if got := m.list.FilterValue(); got != "q" {
			t.Fatalf("got filter %q, want q", got)
		}
		if m.Browsing() {
			t.Fatal("subject shortcuts active while filtering")
		}
		m, _ = m.Update(key(tea.KeyEscape))
		if !m.Browsing() {
			t.Fatal("Escape did not clear filter")
		}
	})
	t.Run("detail wraps scrolls and preserves selection across resizing and reload", func(t *testing.T) {
		service := thought.NewService(testutils.NewFakeThoughtStore())
		id := int64(1)
		created, err := service.Create(context.Background(), "u", "first\n"+strings.Repeat("body\n", 30)+"last", &id, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		m := New(context.Background(), "u", service, logging.Nop())
		cmd := m.Open(id)
		m, _ = m.Update(cmd())
		m, _ = m.Update(key(tea.KeyDown))
		m, cmd = m.Update(key(tea.KeyEnter))
		m, _ = m.Update(cmd())
		m.Resize(20, 10)
		if m.viewport.Width() != 20 || m.viewport.Height() != 5 || !m.viewport.SoftWrap {
			t.Fatal("detail resize lost bounds or wrapping")
		}
		for range 8 {
			m, _ = m.Update(key(tea.KeyPgDown))
		}
		if !strings.Contains(m.View(), "last") {
			t.Fatal("detail did not scroll to end")
		}
		m, cmd = m.Update(key('r'))
		m, _ = m.Update(cmd())
		if m.selected.ThoughtID != created.ThoughtID || !strings.Contains(m.View(), "first") {
			t.Fatal("reload lost selected thought")
		}
		m, _ = m.Update(key(tea.KeyEscape))
		if m.list.Index() != 1 {
			t.Fatal("detail return lost list selection")
		}
		m.Resize(0, 0)
		_ = m.View()
	})
	t.Run("previews are bounded Unicode-safe single lines", func(t *testing.T) {
		r := row{kind: rowRecord, item: data.Thought{ThoughtID: 1, Thought: strings.Repeat("界\n", 100)}}
		got := r.Title()
		if !utf8.ValidString(got) || utf8.RuneCountInString(got) != 81 || strings.Contains(got, "\n") {
			t.Fatalf("invalid preview %q", got)
		}
	})
	t.Run("creates multiline thought and retrieves it from refreshed list", func(t *testing.T) {
		service := thought.NewService(testutils.NewFakeThoughtStore())
		m := New(context.Background(), "u", service, logging.Nop())
		cmd := m.Open(7)
		m, _ = m.Update(cmd())
		m, _ = m.Update(key(tea.KeyEnter))
		body := "first\n" + strings.Repeat("line\n", 120) + "last"
		m, _ = m.Update(tea.PasteMsg{Content: body})
		if m.input.Value() != body {
			t.Fatalf("editor changed or truncated input")
		}
		m, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
		if _, again := m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl})); again != nil {
			t.Fatal("duplicate save command")
		}
		m, cmd = m.Update(cmd())
		if cmd == nil {
			t.Fatal("missing count invalidation")
		}
		if _, ok := cmd().(ChangedMsg); !ok {
			t.Fatal("missing changed message")
		}
		if m.selected == nil || m.selected.Thought != body || *m.selected.SubjectID != 7 || m.selected.ObservedAt.IsZero() {
			t.Fatalf("unexpected created thought: %#v", m.selected)
		}
		m, cmd = m.Update(key(tea.KeyEscape))
		m, _ = m.Update(cmd())
		m, _ = m.Update(key(tea.KeyDown))
		m, cmd = m.Update(key(tea.KeyEnter))
		m, _ = m.Update(cmd())
		if m.selected.Thought != body || !strings.Contains(m.View(), "first") {
			t.Fatal("detail lost text")
		}
	})
	t.Run("ignores results from an earlier subject", func(t *testing.T) {
		m := New(context.Background(), "u", thought.NewService(testutils.NewFakeThoughtStore()), logging.Nop())
		old := m.Open(1)
		current := m.Open(2)
		m, _ = m.Update(old())
		if !m.loading || (m.subjectID == nil || *m.subjectID != 2) {
			t.Fatal("stale result changed active request")
		}
		m, _ = m.Update(current())
		if m.loading {
			t.Fatal("current result not handled")
		}
	})
	t.Run("deferred creation keeps its original subject and cannot replace a reopened session", func(t *testing.T) {
		service := thought.NewService(testutils.NewFakeThoughtStore())
		m := New(context.Background(), "u", service, logging.Nop())
		m.Open(1)
		cmd := m.createThought("original draft")
		current := m.Open(2)
		result := cmd().(Result)
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.item.SubjectID == nil || *result.item.SubjectID != 1 || result.item.Thought != "original draft" {
			t.Fatal("deferred creation lost its original subject or draft")
		}
		m, _ = m.Update(result)
		if !m.loading || (m.subjectID == nil || *m.subjectID != 2) || m.selected != nil || m.stale {
			t.Fatal("old creation result changed the reopened session")
		}
		m, _ = m.Update(current())
		if m.loading || len(m.list.Items()) != 1 {
			t.Fatal("new subject did not retain its empty thought list")
		}
	})
	t.Run("validation preserves input and shows trusted guidance", func(t *testing.T) {
		m := New(context.Background(), "u", thought.NewService(testutils.NewFakeThoughtStore()), logging.Nop())
		cmd := m.Open(1)
		m, _ = m.Update(cmd())
		m, _ = m.Update(key(tea.KeyEnter))
		m.input.SetValue("   ")
		m, cmd = m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
		m, _ = m.Update(cmd())
		if m.input.Value() != "   " || !m.input.Focused() || !strings.Contains(m.View(), "must be provided") {
			t.Fatal("validation did not preserve editor and guidance")
		}
	})
}

func TestModel_Timestamps(t *testing.T) {
	t.Run("list and detail show Pacific time without changing thought data", func(t *testing.T) {
		observedAt := time.Date(2026, time.July, 3, 0, 4, 5, 0, time.UTC)
		item := data.Thought{ThoughtID: 7, Thought: "A thought to keep intact", ObservedAt: observedAt}
		r := row{kind: rowRecord, item: item}
		if got, want := r.Description(), "Jul 2, 2026 5:04 PM PDT"; got != want {
			t.Errorf("got description %q, want %q", got, want)
		}
		m := New(context.Background(), "u", nil, logging.Nop())
		m.screen = detail
		m.selected = &item
		m.viewport.SetContent(item.Thought)
		view := m.View()
		for _, want := range []string{"Thought 7 • Jul 2, 2026 5:04:05 PM PDT", item.Thought} {
			if !strings.Contains(view, want) {
				t.Errorf("got view %q, want to contain %q", view, want)
			}
		}
		if got := m.selected.ObservedAt; got != observedAt {
			t.Errorf("got observed time %v, want original UTC value %v", got, observedAt)
		}
	})
}
