package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
)

type editorPreferencesStub struct {
	preferences data.EditorPreferences
	err         error
	saves       int
}

func (s *editorPreferencesStub) Load(context.Context) (data.EditorPreferences, error) {
	return s.preferences, s.err
}
func (s *editorPreferencesStub) Save(_ context.Context, p data.EditorPreferences) error {
	if s.err == nil {
		s.preferences = p
		s.saves++
	}
	return s.err
}

func TestModel_EditorPreferences(t *testing.T) {
	t.Run("native Options loads adjusts and saves Editor preferences", func(t *testing.T) {
		m := newRootTestModel()
		store := &editorPreferencesStub{preferences: data.DefaultEditorPreferences()}
		m.editorPreferences.service = store
		m.selectedEntity = entityOptions
		m, _ = rootUpdate(m, enterKey())
		if !m.options.open || !strings.Contains(m.View().Content, "Editor") {
			t.Fatal("native Options missing Editor")
		}
		m = runModelCommand(t, m, enterKey())
		if !m.editorPreferences.open || !strings.Contains(m.View().Content, "2.0 seconds") {
			t.Fatal("preferences did not load")
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyRight})
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyTab})
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyRight})
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyTab})
		m = runModelCommand(t, m, enterKey())
		if store.saves != 1 || !store.preferences.HighlightCurrentLine || store.preferences.SpeechPauseMS != 2500 || !strings.Contains(m.View().Content, "Saved.") {
			t.Fatal("settings did not persist")
		}
	})
	t.Run("failed saves keep draft preferences and expose only fixed feedback", func(t *testing.T) {
		m := newRootTestModel()
		store := &editorPreferencesStub{preferences: data.DefaultEditorPreferences()}
		m.editorPreferences.service = store
		m.options.open = true
		m.options.menu = true
		updated, cmd := m.openEditorPreferences()
		m = updated.(Model)
		m, _ = rootUpdate(m, cmd())
		m.editorPreferences.preferences.SpeechPauseMS = 3500
		m.editorPreferences.focus = 2
		store.err = errors.New("private database path")
		m = runModelCommand(t, m, enterKey())
		if !strings.Contains(m.View().Content, "Could not save") || strings.Contains(m.View().Content, "private database") || m.editorPreferences.preferences.SpeechPauseMS != 3500 || store.saves != 0 {
			t.Fatal("save error lost draft or leaked details")
		}
		store.err = nil
		m = runModelCommand(t, m, enterKey())
		if store.preferences.SpeechPauseMS != 3500 {
			t.Fatal("retry did not save draft")
		}
	})
	t.Run("old initial read cannot replace newer saved preferences", func(t *testing.T) {
		m := newRootTestModel()
		m.editorPreferences.service = &editorPreferencesStub{}
		m.editorPreferences.request = 1
		reply := m.readEditorPreferences()()
		m.editorPreferences.request = 2
		m.editorPreferences.preferences = data.EditorPreferences{SpeechPauseMS: 5000}
		m, _ = rootUpdate(m, reply)
		if m.editorPreferences.preferences.SpeechPauseMS != 5000 {
			t.Fatal("stale preferences replaced current settings")
		}
	})
}
