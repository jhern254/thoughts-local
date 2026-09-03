//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/jhern254/go-thoughts/internal/user"
)

func TestSubjectCLIWorkflow_SQLite(t *testing.T) {
	t.Run("creates subject", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)

		stdout, stderr, err := runSubjectsCLI(t, dsn, "create", "  learn Go  ")

		if err != nil {
			t.Fatalf("run CLI: %v: %s", err, stderr)
		}
		if got, want := stdout, "Created subject 1:   learn Go  \n"; got != want {
			t.Fatalf("got output %q, want %q", got, want)
		}

		localUser := getLocalUser(t, db)
		created, err := subject.NewService(data.NewSQLiteSubjectStore(db)).Get(context.Background(), localUser.UserID, 1)
		if err != nil {
			t.Fatal(err)
		}
		if created.SubjectName != "  learn Go  " {
			t.Fatalf("got subject name %q", created.SubjectName)
		}

		stdout, stderr, err = runSubjectsCLI(t, dsn, "create", "writing")
		if err != nil {
			t.Fatalf("run CLI again: %v: %s", err, stderr)
		}
		if got, want := stdout, "Created subject 2: writing\n"; got != want {
			t.Fatalf("got second output %q, want %q", got, want)
		}
		reused := getLocalUser(t, db)
		if reused.UserID != localUser.UserID {
			t.Fatalf("got second user ID %q, want %q", reused.UserID, localUser.UserID)
		}
		var userCount int
		if err := db.QueryRow("SELECT count(*) FROM users WHERE handle = 'local'").Scan(&userCount); err != nil {
			t.Fatal(err)
		}
		if userCount != 1 {
			t.Fatalf("got %d local users, want 1", userCount)
		}
	})

	t.Run("gets subject", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		localUser := ensureLocalUser(t, db)
		created, err := subject.NewService(data.NewSQLiteSubjectStore(db)).Create(context.Background(), localUser.UserID, "coding")
		if err != nil {
			t.Fatal(err)
		}

		stdout, stderr, err := runSubjectsCLI(t, dsn, "get", "1")

		if err != nil {
			t.Fatalf("run CLI: %v: %s", err, stderr)
		}
		if got, want := stdout, "Subject 1: coding\n"; got != want {
			t.Fatalf("got output %q, want %q", got, want)
		}
		if created.SubjectID != 1 {
			t.Fatalf("got subject ID %d, want 1", created.SubjectID)
		}
	})

	t.Run("lists subjects for user", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		localUser := ensureLocalUser(t, db)
		insertUsers(t, db, "user-2")
		service := subject.NewService(data.NewSQLiteSubjectStore(db))
		ctx := context.Background()
		if _, err := service.Create(ctx, localUser.UserID, "coding"); err != nil {
			t.Fatal(err)
		}
		if _, err := service.Create(ctx, "user-2", "private"); err != nil {
			t.Fatal(err)
		}
		if _, err := service.Create(ctx, localUser.UserID, "writing"); err != nil {
			t.Fatal(err)
		}

		stdout, stderr, err := runSubjectsCLI(t, dsn, "list")

		if err != nil {
			t.Fatalf("run CLI: %v: %s", err, stderr)
		}
		if got, want := stdout, "1\tcoding\n3\twriting\n"; got != want {
			t.Fatalf("got output %q, want %q", got, want)
		}
	})

	t.Run("prints nothing for empty list", func(t *testing.T) {
		_, dsn := openMigratedSQLite(t)

		stdout, stderr, err := runSubjectsCLI(t, dsn, "list")

		if err != nil {
			t.Fatalf("run CLI: %v: %s", err, stderr)
		}
		if stdout != "" {
			t.Fatalf("got unexpected output %q", stdout)
		}
	})

	t.Run("updates subject", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		localUser := ensureLocalUser(t, db)
		service := subject.NewService(data.NewSQLiteSubjectStore(db))
		created, err := service.Create(context.Background(), localUser.UserID, "coding")
		if err != nil {
			t.Fatal(err)
		}

		stdout, stderr, err := runSubjectsCLI(t, dsn, "update", "1", "  Go programming  ")

		if err != nil {
			t.Fatalf("run CLI: %v: %s", err, stderr)
		}
		if got, want := stdout, "Updated subject 1:   Go programming  \n"; got != want {
			t.Fatalf("got output %q, want %q", got, want)
		}
		updated, err := service.Get(context.Background(), localUser.UserID, created.SubjectID)
		if err != nil {
			t.Fatal(err)
		}
		if updated.SubjectName != "  Go programming  " {
			t.Fatalf("got updated subject name %q", updated.SubjectName)
		}
	})

	t.Run("prevents cross-user access and mutation", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "user-1")
		service := subject.NewService(data.NewSQLiteSubjectStore(db))
		created, err := service.Create(context.Background(), "user-1", "coding")
		if err != nil {
			t.Fatal(err)
		}

		for _, args := range [][]string{
			{"get", "1"},
			{"update", "1", "not mine"},
			{"delete", "1"},
		} {
			stdout, _, err := runSubjectsCLI(t, dsn, args...)
			if err == nil {
				t.Fatalf("expected %s error", args[0])
			}
			if stdout != "" {
				t.Fatalf("got unexpected %s output %q", args[0], stdout)
			}
		}
		found, err := service.Get(context.Background(), "user-1", created.SubjectID)
		if err != nil {
			t.Fatal(err)
		}
		if found.SubjectName != "coding" {
			t.Fatalf("got subject name %q after cross-user commands", found.SubjectName)
		}
	})

	t.Run("deletes subject and unlinks linked thoughts", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		localUser := ensureLocalUser(t, db)
		ctx := context.Background()
		subjectService := subject.NewService(data.NewSQLiteSubjectStore(db))
		created, err := subjectService.Create(ctx, localUser.UserID, "coding")
		if err != nil {
			t.Fatal(err)
		}
		thoughtService := thought.NewService(data.NewSQLiteThoughtStore(db))
		linked, err := thoughtService.Create(ctx, localUser.UserID, "keep me", &created.SubjectID, time.Time{})
		if err != nil {
			t.Fatal(err)
		}

		stdout, stderr, err := runSubjectsCLI(t, dsn, "delete", "1")

		if err != nil {
			t.Fatalf("run CLI: %v: %s", err, stderr)
		}
		if got, want := stdout, "Deleted subject 1\n"; got != want {
			t.Fatalf("got output %q, want %q", got, want)
		}
		if _, err := subjectService.Get(ctx, localUser.UserID, created.SubjectID); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got deleted subject error %v, want %v", err, data.ErrRecordNotFound)
		}
		unlinked, err := thoughtService.Get(ctx, localUser.UserID, linked.ThoughtID)
		if err != nil {
			t.Fatal(err)
		}
		if unlinked.SubjectID != nil {
			t.Fatalf("got linked subject ID %#v after deletion", unlinked.SubjectID)
		}
	})
}

func runSubjectsCLI(t *testing.T, dsn string, args ...string) (string, string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	commandArgs := []string{
		"run", "../cmd/thoughts",
		"--db-dsn", dsn,
		"subjects",
	}
	command := exec.CommandContext(ctx, "go", append(commandArgs, args...)...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}

func ensureLocalUser(t *testing.T, db *sql.DB) *data.User {
	t.Helper()
	localUser, err := user.NewService(data.NewSQLiteUserStore(db)).EnsureLocalUser(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return localUser
}

func getLocalUser(t *testing.T, db *sql.DB) *data.User {
	t.Helper()
	localUser, err := data.NewSQLiteUserStore(db).GetUserByHandle(context.Background(), "local")
	if err != nil {
		t.Fatal(err)
	}
	return localUser
}
