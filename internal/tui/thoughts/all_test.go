package thoughts

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
)

func allModel(t *testing.T, count int) Model {
	t.Helper()
	store := testutils.NewFakeThoughtStore()
	for id := 1; id <= count; id++ {
		_, err := store.CreateThought(t.Context(), &data.Thought{UserID: "u", Thought: fmt.Sprintf("body %d", id), ObservedAt: time.Unix(int64(id), 0), CreatedAt: time.Unix(99999999, 0)})
		if err != nil {
			t.Fatal(err)
		}
	}
	m := New(t.Context(), "u", thought.NewService(store), logging.Nop())
	cmd := m.OpenAll()
	m, _ = m.Update(cmd())
	return m
}

func allKey(m Model, code rune) Model {
	m, cmd := m.Update(key(code))
	if cmd != nil {
		m, _ = m.Update(cmd())
	}
	return m
}

func TestModel_AllThoughts(t *testing.T) {
	t.Run("counts records without loaded wording or the Create action", func(t *testing.T) {
		for _, count := range []int{0, 1, 2} {
			m := allModel(t, count)
			label := "thoughts"
			if count == 1 {
				label = "thought"
			}
			want := fmt.Sprintf("\n%d %s\n", count, label)
			if got := ansi.Strip(m.View()); !strings.Contains(got, want) || strings.Contains(got, "loaded") {
				t.Fatalf("got %q, want count %q without loaded wording", got, want)
			}
		}
	})
	t.Run("uses the native picker style and subject color blocks for selected and unselected rows", func(t *testing.T) {
		m := allModel(t, 2)
		name := "Writing"
		m.all.rows[1].item.SubjectName = &name
		var native bytes.Buffer
		list.NewDefaultDelegate().Render(&native, m.list, 0, row{kind: rowCreate, unassigned: true})
		if !strings.HasPrefix(m.View(), native.String()+"\n") {
			t.Fatalf("got picker %q, want native selection %q", m.View(), native.String())
		}
		for range 3 {
			for _, label := range []string{"Writing", "Misc"} {
				badge := m.list.Styles.Title.Render(label)
				if !strings.Contains(m.View(), badge) {
					t.Fatalf("missing subject color block %q in %q", badge, m.View())
				}
			}
			m = allKey(m, tea.KeyDown)
		}
	})
	t.Run("eviction preserves selected ID and its screen position at either edge", func(t *testing.T) {
		m := allModel(t, 230)
		for range 150 {
			m = allKey(m, tea.KeyDown)
		}
		id, position := m.all.rows[m.all.index].item.ThoughtID, m.all.index*summaryLines-m.all.offset
		cursor := m.all.rows[len(m.all.rows)-1].item.Cursor()
		cmd := m.browseThoughts(data.ThoughtPageRequest{Cursor: &cursor}, 0)
		m, _ = m.Update(cmd())
		if m.all.rows[m.all.index].item.ThoughtID != id || m.all.index*summaryLines-m.all.offset != position || len(m.all.rows) != 150 {
			t.Fatal("older merge moved anchor or exceeded window")
		}
		m.all.index = 0
		m.all.keepVisible()
		id, position = m.all.rows[m.all.index].item.ThoughtID, m.all.index*summaryLines-m.all.offset
		cursor = m.all.rows[0].item.Cursor()
		cmd = m.browseThoughts(data.ThoughtPageRequest{Cursor: &cursor, Direction: data.ThoughtsNewer}, 0)
		m, _ = m.Update(cmd())
		if m.all.rows[m.all.index].item.ThoughtID != id || m.all.index*summaryLines-m.all.offset != position || len(m.all.rows) != 151 {
			t.Fatal("newer merge moved anchor or exceeded window plus Create")
		}
	})
	t.Run("renders observation time and subject labels with bounded normal and narrow layouts", func(t *testing.T) {
		m := allModel(t, 2)
		name := "Writing"
		m.all.rows[1].item.SubjectName = &name
		m.all.rows[1].item.ObservedAt = time.Date(2026, 7, 3, 0, 4, 0, 0, time.UTC)
		m.all.rows[2].item.ObservedAt = time.Date(2026, 7, 2, 0, 4, 0, 0, time.UTC)
		for _, width := range []int{80, 40} {
			m.Resize(width, 14)
			want := "│ Create thought…\n│ Write a new thought\n\n  Thought 2 • body 2\n   Writing  • Jul 2, 2026 5:04 PM PDT\n\n  Thought 1 • body 1\n   Misc  • Jul 1, 2026 5:04 PM PDT\n\n2 thoughts\n\n↑/↓: select • PgUp/PgDn: scroll\nEnter: open • Home/r: latest"
			if got := ansi.Strip(m.View()); got != want {
				t.Fatalf("width %d:\ngot %q\nwant %q", width, got, want)
			}
			for _, line := range strings.Split(m.View(), "\n") {
				if ansi.StringWidth(line) > width {
					t.Fatalf("line exceeds width %d: %q", width, line)
				}
			}
		}
		m = allKey(m, tea.KeyDown)
		m.Resize(20, 8)
		if m.all.rows[m.all.index].item.ThoughtID != 2 || !strings.Contains(ansi.Strip(m.View()), "│ Thought 2") {
			t.Fatal("resize lost visible selection")
		}
		m.Resize(0, 0)
		_ = m.View()
	})
	t.Run("styled Unicode rows stay within the viewport without altering their text", func(t *testing.T) {
		m := allModel(t, 1)
		name, preview := strings.Repeat("界", 80), strings.Repeat("語", 80)
		m.all.rows[1].item.SubjectName = &name
		m.all.rows[1].item.Preview = preview
		m = allKey(m, tea.KeyDown)
		for _, width := range []int{80, 40, 20, 2, 1} {
			m.Resize(width, 14)
			rows := strings.Split(m.View(), "\n")[:m.all.height]
			for _, line := range rows {
				if ansi.StringWidth(line) > width {
					t.Fatalf("width %d exceeded by styled line %q", width, line)
				}
			}
			if m.all.rows[m.all.index].item.Preview != preview || *m.all.rows[m.all.index].item.SubjectName != name {
				t.Fatal("rendering changed stored display metadata")
			}
		}
	})
	t.Run("page keys move within the viewport and suppress duplicate pending requests", func(t *testing.T) {
		m := allModel(t, 130)
		m.Resize(60, 17)
		m = allKey(m, tea.KeyPgDown)
		if m.all.index != 4 {
			t.Fatalf("got selection %d, want four records down", m.all.index)
		}
		m = allKey(m, tea.KeyPgUp)
		if m.all.index != 0 {
			t.Fatal("page up did not return to Create")
		}
		for range 50 {
			m = allKey(m, tea.KeyDown)
		}
		m, cmd := m.Update(key(tea.KeyDown))
		if cmd == nil || !strings.Contains(m.View(), "Loading…") {
			t.Fatal("boundary did not request next batch")
		}
		before := m.View()
		m, duplicate := m.Update(key(tea.KeyDown))
		if duplicate != nil || m.View() != before {
			t.Fatal("pending boundary request duplicated or moved selection")
		}
		m, _ = m.Update(cmd())
		if m.all.rows[m.all.index].item.ThoughtID != 80 {
			t.Fatal("boundary did not select next record")
		}
	})
	t.Run("loads bounded batches in both directions and restores detail position", func(t *testing.T) {
		m := allModel(t, 230)
		if len(m.all.rows) != 51 || m.all.rows[0].kind != rowCreate {
			t.Fatal("missing first batch or typed Create row")
		}
		for range 201 {
			m = allKey(m, tea.KeyDown)
		}
		if len(m.all.rows) > 150 || m.all.rows[m.all.index].item.ThoughtID != 30 {
			t.Fatalf("unexpected older window: %d rows, selection %+v", len(m.all.rows), m.all.rows[m.all.index])
		}
		index, offset := m.all.index, m.all.offset
		m = allKey(m, tea.KeyEnter)
		if m.selected == nil || m.selected.ThoughtID != 30 {
			t.Fatal("did not fetch full thought")
		}
		m = allKey(m, tea.KeyEscape)
		if m.all.index != index || m.all.offset != offset {
			t.Fatal("detail return moved list")
		}
		for range 201 {
			m = allKey(m, tea.KeyUp)
		}
		if m.all.rows[m.all.index].kind != rowCreate || len(m.all.rows) > 151 {
			t.Fatal("reverse scrolling did not restore Create within bounded window")
		}
	})
	t.Run("Home supersedes an actual pending reply and slash does not filter a partial list", func(t *testing.T) {
		m := allModel(t, 120)
		for range 50 {
			m = allKey(m, tea.KeyDown)
		}
		m, cmd := m.Update(key(tea.KeyDown))
		old := cmd()
		m, cmd = m.Update(key(tea.KeyHome))
		m, _ = m.Update(cmd())
		m, _ = m.Update(old)
		if len(m.all.rows) != 51 || m.all.index != 0 || m.loading {
			t.Fatal("old reply replaced refreshed list")
		}
		m = allKey(m, '/')
		if !m.Browsing() || m.list.SettingFilter() {
			t.Fatal("All Thoughts enabled partial filtering")
		}
	})
	t.Run("empty list keeps Create and editor retains complete input", func(t *testing.T) {
		m := allModel(t, 0)
		if !strings.Contains(m.View(), "\n0 thoughts\n") {
			t.Fatal(m.View())
		}
		m = allKey(m, tea.KeyEnter)
		body := "q\n" + strings.Repeat("界", 100000)
		m, _ = m.Update(tea.PasteMsg{Content: body})
		if m.input.Value() != body {
			t.Fatal("editor shortened draft")
		}
		m, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
		m, _ = m.Update(cmd())
		if m.selected.SubjectID != nil || m.selected.Thought != body {
			t.Fatal("Create altered unassigned thought")
		}
		m = allKey(m, tea.KeyEscape)
		if len(m.all.rows) != 2 || m.all.index != 0 {
			t.Fatal("creation did not reload latest batch")
		}
	})
}
