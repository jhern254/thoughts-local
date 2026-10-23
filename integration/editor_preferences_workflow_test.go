//go:build integration

package integration_test

import (
	"database/sql"
	"testing"

	"github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/data"
)

func TestEditorPreferencesWorkflow_SQLite(t *testing.T) {
	t.Run("defaults and preferences survive reopening", func(t *testing.T) {
		_, dsn := openMigratedSQLite(t)
		runtime, err := application.Open(t.Context(), dsn)
		if err != nil {
			t.Fatal(err)
		}
		service := runtime.EditorPreferences()
		got, err := service.Load(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if want := data.DefaultEditorPreferences(); got != want {
			t.Fatalf("got %+v, want %+v", got, want)
		}
		want := data.EditorPreferences{HighlightCurrentLine: true, SpeechPauseMS: 3500}
		if err := service.Save(t.Context(), want); err != nil {
			t.Fatal(err)
		}
		if err := runtime.Close(); err != nil {
			t.Fatal(err)
		}
		reopened, err := application.Open(t.Context(), dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		got, err = reopened.EditorPreferences().Load(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})
	t.Run("isolates users and rejects invalid saved settings", func(t *testing.T) {
		_, dsn := openMigratedSQLite(t)
		runtime, err := application.Open(t.Context(), dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer runtime.Close()
		if err := runtime.EditorPreferences().Save(t.Context(), data.EditorPreferences{SpeechPauseMS: 501}); err == nil {
			t.Fatal("accepted invalid pause")
		}
		db, err := sql.Open("sqlite", dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if _, err := db.ExecContext(t.Context(), "INSERT INTO users(user_id) VALUES ('another-user')"); err != nil {
			t.Fatal(err)
		}
		store := data.NewSQLiteEditorPreferencesStore(db)
		if err := runtime.EditorPreferences().Save(t.Context(), data.EditorPreferences{SpeechPauseMS: 0}); err != nil {
			t.Fatal(err)
		}
		got, err := store.Load(t.Context(), "another-user")
		if err != nil {
			t.Fatal(err)
		}
		if got != data.DefaultEditorPreferences() {
			t.Fatalf("got %+v, want independent defaults", got)
		}
		if err := store.Save(t.Context(), "another-user", data.EditorPreferences{HighlightCurrentLine: true, SpeechPauseMS: 500}); err != nil {
			t.Fatal(err)
		}
		current, err := runtime.EditorPreferences().Load(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if current.SpeechPauseMS != 0 || current.HighlightCurrentLine {
			t.Fatalf("got %+v, want unchanged first user", current)
		}
	})
}
