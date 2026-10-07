package tui

import (
	"strings"
	"testing"
)

func TestModel_Options(t *testing.T) {
	t.Run("opens browser options without leaving or reloading the timeline", func(t *testing.T) {
		m := newRootTestModel()
		m, _ = rootUpdate(m, BrowserOptionsEnabledMsg{})
		m, _ = rootUpdate(m, runeKey('l'))
		if m.selectedEntity != entityOptions {
			t.Fatalf("selected = %v, want Options", m.selectedEntity)
		}
		before := m.View().Content
		next, cmd := rootUpdate(m, enterKey())
		if cmd == nil {
			t.Fatal("missing browser options request")
		}
		if _, ok := cmd().(OpenBrowserOptionsMsg); !ok {
			t.Fatal("wrong options request")
		}
		if next.screen != screenEvents || next.View().Content != before {
			t.Fatal("opening Options changed the underlying timeline")
		}
	})
	t.Run("native options explains browser presentation and returns", func(t *testing.T) {
		m := newRootTestModel()
		m, _ = rootUpdate(m, runeKey('l'))
		m, _ = rootUpdate(m, enterKey())
		if !strings.Contains(m.View().Content, "browser mode") {
			t.Fatal("missing native explanation")
		}
		m, cmd := rootUpdate(m, runeKey('q'))
		if cmd != nil {
			t.Fatal("Options back navigation returned an exit command")
		}
		if m.screen != screenEvents || m.selectedEntity != entityOptions {
			t.Fatal("did not preserve Options selection on return")
		}
	})
}
