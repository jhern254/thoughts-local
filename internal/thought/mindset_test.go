package thought

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

type mindsetStoreStub struct {
	Store
	called bool
	err    error
}

func (s *mindsetStoreStub) SetThoughtMindset(ctx context.Context, user string, id int64, active bool, at time.Time) (bool, error) {
	s.called = true
	if user != "owner" || id != 7 || !active || at.Unix() != 100 || at.Location() != time.UTC {
		panic("lost mindset action")
	}
	return active, s.err
}
func (s *mindsetStoreStub) ListMindsetsInRange(ctx context.Context, user string, from, until time.Time) ([]data.MindsetThought, error) {
	s.called = true
	if user != "owner" || from.Unix() != 100 || until.Unix() != 200 {
		panic("lost mindset range")
	}
	return []data.MindsetThought{{ThoughtID: 7}}, s.err
}
func TestService_Mindset(t *testing.T) {
	t.Run("passes captured time and preserves store errors", func(t *testing.T) {
		want := errors.New("store failure")
		store := &mindsetStoreStub{err: want}
		active, err := NewService(store).SetMindset(t.Context(), "owner", 7, true, time.Unix(100, 0))
		if !active || err != want || !store.called {
			t.Fatalf("got %v, %v, called=%v", active, err, store.called)
		}
	})
	t.Run("rejects absent action time before storage", func(t *testing.T) {
		store := &mindsetStoreStub{}
		_, err := NewService(store).SetMindset(t.Context(), "owner", 7, true, time.Time{})
		if err == nil || store.called {
			t.Fatalf("got err=%v called=%v, want validation without storage", err, store.called)
		}
	})
	t.Run("passes range and preserves store errors", func(t *testing.T) {
		want := errors.New("store failure")
		store := &mindsetStoreStub{err: want}
		rows, err := NewService(store).ListMindsetsInRange(t.Context(), "owner", time.Unix(100, 0), time.Unix(200, 0))
		if len(rows) != 1 || rows[0].ThoughtID != 7 || err != want {
			t.Fatalf("got %v, %v, want supplied rows and error", rows, err)
		}
	})
	t.Run("rejects reversed range before storage", func(t *testing.T) {
		store := &mindsetStoreStub{}
		_, err := NewService(store).ListMindsetsInRange(t.Context(), "owner", time.Unix(200, 0), time.Unix(100, 0))
		if err == nil || store.called {
			t.Fatalf("got err=%v called=%v, want validation without storage", err, store.called)
		}
	})
}
