package editorprefs

import (
	"context"
	"errors"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
)

type preferenceStore struct {
	saved bool
	err   error
	user  string
}

func (s *preferenceStore) Load(_ context.Context, user string) (data.EditorPreferences, error) {
	s.user = user
	return data.DefaultEditorPreferences(), s.err
}
func (s *preferenceStore) Save(_ context.Context, user string, _ data.EditorPreferences) error {
	s.user = user
	s.saved = true
	return s.err
}
func TestService_Save(t *testing.T) {
	for _, pause := range []int{-1, 1, 499, 501, 10001} {
		t.Run("rejects invalid pause", func(t *testing.T) {
			store := &preferenceStore{}
			service := NewService(store, "user")
			if err := service.Save(t.Context(), data.EditorPreferences{SpeechPauseMS: pause}); !errors.Is(err, ErrInvalidPause) || store.saved {
				t.Fatal("invalid preferences reached store")
			}
		})
	}
	for _, pause := range []int{0, 500, 2000, 10000} {
		t.Run("persists valid pause for current user", func(t *testing.T) {
			store := &preferenceStore{}
			service := NewService(store, "user")
			if err := service.Save(t.Context(), data.EditorPreferences{SpeechPauseMS: pause}); err != nil {
				t.Fatal(err)
			}
			if !store.saved || store.user != "user" {
				t.Fatal("wrong preference owner")
			}
		})
	}
	t.Run("preserves store errors", func(t *testing.T) {
		cause := errors.New("private cause")
		service := NewService(&preferenceStore{err: cause}, "user")
		if err := service.Save(t.Context(), data.DefaultEditorPreferences()); !errors.Is(err, cause) {
			t.Fatal("lost error")
		}
		if _, err := service.Load(t.Context()); !errors.Is(err, cause) {
			t.Fatal("lost error")
		}
	})
}
