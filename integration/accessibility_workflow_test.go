//go:build integration

package integration_test

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/visual"
)

func TestAccessibilityWorkflow_SQLite(t *testing.T) {
	t.Run("defaults are off and saved preferences survive reopening for only their user", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		if _, err := db.ExecContext(t.Context(), "INSERT INTO users(user_id) VALUES ('u'),('other')"); err != nil {
			t.Fatal(err)
		}
		store := data.NewSQLiteVisualStore(db)
		settings, err := store.Load(t.Context(), "u")
		if err != nil {
			t.Fatal(err)
		}
		if settings.ThickerCardBorders || settings.HeavierText {
			t.Fatalf("got %+v, want accessibility defaults off", settings)
		}
		settings.ThickerCardBorders, settings.HeavierText = true, true
		if err := store.Save(t.Context(), "u", settings); err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		store = data.NewSQLiteVisualStore(openSQLite(t, dsn))
		got, err := store.Load(t.Context(), "u")
		if err != nil {
			t.Fatal(err)
		}
		if got != settings {
			t.Fatalf("got %+v, want %+v", got, settings)
		}
		other, err := store.Load(t.Context(), "other")
		if err != nil {
			t.Fatal(err)
		}
		if other.ThickerCardBorders || other.HeavierText {
			t.Fatalf("other user got %+v, want defaults off", other)
		}
	})
	t.Run("background import updates and removal retain accessibility preferences", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		if _, err := db.ExecContext(t.Context(), "INSERT INTO users(user_id) VALUES ('u')"); err != nil {
			t.Fatal(err)
		}
		service := visual.NewService(data.NewSQLiteVisualStore(db), "u", t.TempDir())
		if _, err := service.UpdateAccessibility(t.Context(), true, true); err != nil {
			t.Fatal(err)
		}
		var source bytes.Buffer
		if err := png.Encode(&source, image.NewRGBA(image.Rect(0, 0, 12, 8))); err != nil {
			t.Fatal(err)
		}
		if _, _, err := service.Import(t.Context(), &source, 42, data.DefaultBackgroundFraming(), visual.VideoQualityEfficient); err != nil {
			t.Fatal(err)
		}
		if _, err := service.UpdateSettings(t.Context(), 50, data.DefaultBackgroundFraming(), 100, false, visual.VideoQualityEfficient); err != nil {
			t.Fatal(err)
		}
		if _, _, err := service.Remove(t.Context()); err != nil {
			t.Fatal(err)
		}
		got, err := service.Load(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if !got.ThickerCardBorders || !got.HeavierText || got.BackgroundAsset != "" {
			t.Fatalf("got %+v, want retained accessibility and no background", got)
		}
	})
	for _, assignment := range []string{"thicker_card_borders=2", "thicker_card_borders=-1", "thicker_card_borders=0.5", "heavier_text=2", "heavier_text=NULL", "heavier_text=0.5"} {
		t.Run("rejects "+assignment, func(t *testing.T) {
			db, _ := openMigratedSQLite(t)
			if _, err := db.ExecContext(t.Context(), "INSERT INTO users(user_id) VALUES ('u'); INSERT INTO visual(user_id) VALUES ('u')"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(t.Context(), "UPDATE visual SET "+assignment); err == nil {
				t.Fatalf("got nil error for %s, want constraint failure", assignment)
			}
		})
	}
}
