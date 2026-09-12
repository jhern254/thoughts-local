package thoughts

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
)

func thoughtEditor(t *testing.T) Model {
	t.Helper()
	m := New(context.Background(), "u", thought.NewService(testutils.NewFakeThoughtStore()), logging.Nop())
	cmd := m.Open(1)
	m, _ = m.Update(cmd())
	m, _ = m.Update(key(tea.KeyEnter))
	return m
}

func TestModel_InputIntegrity(t *testing.T) {
	t.Run("preserves and saves one million Unicode characters at the supported line boundary", func(t *testing.T) {
		m := thoughtEditor(t)
		body := strings.Repeat(strings.Repeat("界", 99)+"\n", 9999) + strings.Repeat("界", 100)
		if utf8.RuneCountInString(body) != 1000000 {
			t.Fatal("incorrect domain boundary fixture")
		}
		m, _ = m.Update(tea.PasteMsg{Content: body})
		if m.input.Value() != body {
			t.Fatal("supported input was shortened")
		}
		// Narrowing the display must not lower the supported content capacity.
		m.Resize(24, 12)
		if m.input.Value() != body {
			t.Fatal("resize altered the draft")
		}
		m, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
		m, _ = m.Update(cmd())
		if m.err != nil || m.selected == nil || m.selected.Thought != body {
			t.Fatal("domain-valid complete draft was not saved")
		}
	})
	t.Run("preserves an over-domain-limit single line for service validation", func(t *testing.T) {
		m := thoughtEditor(t)
		body := strings.Repeat("x", 1000001)
		m, _ = m.Update(tea.PasteMsg{Content: body})
		if m.input.Value() != body {
			t.Fatal("editor imposed a character limit")
		}
		m, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 's', Mod: tea.ModCtrl}))
		m, _ = m.Update(cmd())
		var validation *thought.ValidationError
		if !errors.As(m.err, &validation) || m.input.Value() != body || !m.input.Focused() {
			t.Fatal("service validation lost complete draft")
		}
	})
	t.Run("rejects whole edits crossing the line ceiling including trailing newlines", func(t *testing.T) {
		body := strings.Repeat("x\n", 9999) + "tail"
		for _, msg := range []tea.Msg{key(tea.KeyEnter), tea.KeyPressMsg(tea.Key{Code: 'm', Mod: tea.ModCtrl}), tea.PasteMsg{Content: "\n"}, tea.PasteMsg{Content: "prefix\nnew line"}, tea.KeyPressMsg(tea.Key{Code: 'x', Text: "prefix\nnew line"})} {
			m := thoughtEditor(t)
			m.input.SetValue(body)
			m.input.CursorStart()
			line, column := m.input.Line(), m.input.Column()
			m, _ = m.Update(msg)
			if m.input.Value() != body || m.input.Line() != line || m.input.Column() != column || !strings.Contains(m.inputWarning, "10,000 lines") {
				t.Fatalf("edit %T changed draft/cursor or lacked rejection feedback", msg)
			}
			m, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: 'z', Text: "z"}))
			if m.input.Value() != strings.TrimSuffix(body, "tail")+"ztail" || m.inputWarning != "" {
				t.Fatal("supported edit after rejection did not recover")
			}
		}
	})
	t.Run("accounts for removed selection before checking replacement line count", func(t *testing.T) {
		m := thoughtEditor(t)
		body := strings.Repeat("x\n", 9999) + "tail"
		m.input.SetValue(body)
		m.input.SelectAll()
		replacement := strings.Repeat("y\n", 9999) + "end"
		m, _ = m.Update(tea.PasteMsg{Content: replacement})
		if m.input.Value() != replacement || m.inputWarning != "" {
			t.Fatal("supported selection replacement was rejected or changed")
		}
	})
	t.Run("rejects a valid million-character paste exceeding widget lines without replacing selection", func(t *testing.T) {
		m := thoughtEditor(t)
		m, _ = m.Update(tea.PasteMsg{Content: "existing draft"})
		m.input.SelectAll()
		body := strings.Repeat("x\n", 499999) + "xy"
		if utf8.RuneCountInString(body) != 1000000 {
			t.Fatal("fixture must exercise the domain boundary")
		}
		m, cmd := m.Update(tea.PasteMsg{Content: body})
		if got := m.input.Value(); got != "existing draft" {
			t.Fatalf("got %d draft characters, want unchanged existing draft", utf8.RuneCountInString(got))
		}
		if !m.input.HasSelection() || m.input.SelectedText() != "existing draft" || !m.input.Focused() {
			t.Fatal("rejected paste changed selection or focus")
		}
		if cmd != nil || !strings.Contains(m.View(), "Input rejected: this editor supports at most 10,000 lines. Draft unchanged.") {
			t.Fatal("unsupported paste was not explicitly rejected")
		}
	})
	for _, tt := range []struct{ name, body string }{{"tab", "a\tb"}, {"CRLF", "a\r\nb"}, {"NUL", "a\x00b"}, {"escape", "a\x1bb"}, {"replacement rune", "a\uFFFDb"}, {"invalid UTF-8", string([]byte{'a', 0xff, 'b'})}} {
		t.Run("rejects lossy widget sanitization of "+tt.name, func(t *testing.T) {
			for _, msg := range []tea.Msg{tea.PasteMsg{Content: tt.body}, tea.KeyPressMsg(tea.Key{Code: 'a', Text: tt.body})} {
				m := thoughtEditor(t)
				m.input.SetValue("draft")
				m.input.SelectAll()
				m, _ = m.Update(msg)
				if m.input.Value() != "draft" || m.input.SelectedText() != "draft" || !strings.Contains(m.View(), "Input rejected") {
					t.Fatal("lossy paste changed draft or lacked explicit feedback")
				}
			}
		})
	}
}

func TestModel_ClipboardIntegrity(t *testing.T) {
	t.Run("keeps the selection until a supported clipboard result arrives", func(t *testing.T) {
		m := thoughtEditor(t)
		m.input.SetValue("draft")
		m.input.SelectAll()
		m, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: 'v', Mod: tea.ModCtrl}))
		if cmd == nil || m.input.Value() != "draft" || m.input.SelectedText() != "draft" {
			t.Fatal("clipboard shortcut prematurely changed draft")
		}
		m, _ = m.Update(clipboardResult{request: m.request, content: "replacement\ncomplete"})
		if m.input.Value() != "replacement\ncomplete" {
			t.Fatal("supported clipboard content was not preserved")
		}
	})
	t.Run("clipboard read errors and unsupported contents preserve draft without logging private data", func(t *testing.T) {
		for _, result := range []clipboardResult{{content: "private-marker\t"}, {err: errors.New("private-marker")}} {
			var logs bytes.Buffer
			m := thoughtEditor(t)
			logger, err := logging.New(&logs, "test", "info")
			if err != nil {
				t.Fatal(err)
			}
			m.logger = logger
			m.input.SetValue("draft")
			m.input.SelectAll()
			result.request = m.request
			m, _ = m.Update(result)
			if m.input.Value() != "draft" || m.input.SelectedText() != "draft" || m.inputWarning == "" {
				t.Fatal("failed clipboard operation changed draft or lacked feedback")
			}
			if logs.Len() != 0 || strings.Contains(m.View(), "private-marker") {
				t.Fatal("clipboard failure leaked private data")
			}
		}
	})
	t.Run("empty pastes leave selected text intact", func(t *testing.T) {
		m := thoughtEditor(t)
		m.input.SetValue("draft")
		m.input.SelectAll()
		m, _ = m.Update(tea.PasteMsg{})
		m, _ = m.Update(clipboardResult{request: m.request})
		if m.input.Value() != "draft" || m.input.SelectedText() != "draft" {
			t.Fatal("empty paste deleted selection")
		}
	})
	t.Run("ignores clipboard results from a cancelled draft", func(t *testing.T) {
		m := thoughtEditor(t)
		request := m.request
		m, _ = m.Update(key(tea.KeyEscape))
		m, _ = m.Update(key(tea.KeyEnter))
		m.input.SetValue("new draft")
		m, _ = m.Update(clipboardResult{request: request, content: "old clipboard"})
		if m.input.Value() != "new draft" {
			t.Fatal("stale clipboard result changed new draft")
		}
	})
}
