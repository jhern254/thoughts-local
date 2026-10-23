// Package editorprefs owns saved editing preferences, independently of any renderer.
package editorprefs

import (
	"context"
	"errors"

	"github.com/jhern254/go-thoughts/internal/data"
)

var ErrInvalidPause = errors.New("speech pause must be off or 0.5–10 seconds in 0.5-second steps")

type Store interface {
	Load(context.Context, string) (data.EditorPreferences, error)
	Save(context.Context, string, data.EditorPreferences) error
}
type Service struct {
	store  Store
	userID string
}

func NewService(store Store, userID string) *Service { return &Service{store: store, userID: userID} }
func (s *Service) Load(ctx context.Context) (data.EditorPreferences, error) {
	return s.store.Load(ctx, s.userID)
}
func (s *Service) Save(ctx context.Context, preferences data.EditorPreferences) error {
	if preferences.SpeechPauseMS < 0 || preferences.SpeechPauseMS > 10000 || preferences.SpeechPauseMS%500 != 0 {
		return ErrInvalidPause
	}
	return s.store.Save(ctx, s.userID, preferences)
}
