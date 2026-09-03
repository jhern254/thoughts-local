//go:build integration

package integration_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/user"
)

func TestUserWorkflow_SQLite(t *testing.T) {
	t.Run("creates local user on first bootstrap", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		service := user.NewService(data.NewSQLiteUserStore(db))

		created, err := service.EnsureLocalUser(context.Background())

		if err != nil {
			t.Fatal(err)
		}
		if _, err := uuid.Parse(created.UserID); err != nil {
			t.Fatalf("got user ID %q: %v", created.UserID, err)
		}
		if created.Handle == nil || *created.Handle != "local" || created.Version != 1 {
			t.Fatalf("got local user %#v", created)
		}
		if created.CreatedAt.IsZero() || created.UpdatedAt.IsZero() {
			t.Fatalf("got timestamps %v and %v", created.CreatedAt, created.UpdatedAt)
		}
		assertLocalUserCount(t, db, 1)
	})

	t.Run("reuses local user without creating duplicates", func(t *testing.T) {
		db, _ := openMigratedSQLite(t)
		service := user.NewService(data.NewSQLiteUserStore(db))
		ctx := context.Background()

		first, err := service.EnsureLocalUser(ctx)
		if err != nil {
			t.Fatal(err)
		}
		second, err := service.EnsureLocalUser(ctx)
		if err != nil {
			t.Fatal(err)
		}

		assertUserEqual(t, second, first)
		assertLocalUserCount(t, db, 1)
	})

	t.Run("persists local user after reopening database", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		first, err := user.NewService(data.NewSQLiteUserStore(db)).EnsureLocalUser(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}

		reopened := openSQLite(t, dsn)
		second, err := user.NewService(data.NewSQLiteUserStore(reopened)).EnsureLocalUser(context.Background())
		if err != nil {
			t.Fatal(err)
		}

		assertUserEqual(t, second, first)
		assertLocalUserCount(t, reopened, 1)
	})
}

func assertLocalUserCount(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT count(*) FROM users WHERE handle = 'local'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != want {
		t.Fatalf("got %d local users, want %d", count, want)
	}
}

func assertUserEqual(t *testing.T, got, want *data.User) {
	t.Helper()
	if got.UserID != want.UserID || got.Version != want.Version {
		t.Fatalf("got user %#v, want %#v", got, want)
	}
	if got.Handle == nil || want.Handle == nil || *got.Handle != *want.Handle {
		t.Fatalf("got handle %#v, want %#v", got.Handle, want.Handle)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) || !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Fatalf("got timestamps %v and %v, want %v and %v", got.CreatedAt, got.UpdatedAt, want.CreatedAt, want.UpdatedAt)
	}
}
