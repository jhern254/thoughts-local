//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/thought"
)

func TestSubjectCLIWorkflow_SQLite(t *testing.T) {
	t.Run("creates subject", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "user-1")

		stdout, stderr, err := runSubjectsCLI(t, dsn, "user-1", "create", "  learn Go  ")

		if err != nil {
			t.Fatalf("run CLI: %v: %s", err, stderr)
		}
		if got, want := stdout, "Created subject 1:   learn Go  \n"; got != want {
			t.Fatalf("got output %q, want %q", got, want)
		}

		created, err := subject.NewService(data.NewSQLiteSubjectStore(db)).Get(context.Background(), "user-1", 1)
		if err != nil {
			t.Fatal(err)
		}
		if created.SubjectName != "  learn Go  " {
			t.Fatalf("got subject name %q", created.SubjectName)
		}
	})

	t.Run("gets subject", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "user-1")
		created, err := subject.NewService(data.NewSQLiteSubjectStore(db)).Create(context.Background(), "user-1", "coding")
		if err != nil {
			t.Fatal(err)
		}

		stdout, stderr, err := runSubjectsCLI(t, dsn, "user-1", "get", "1")

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
		insertUsers(t, db, "user-1", "user-2", "user-3")
		service := subject.NewService(data.NewSQLiteSubjectStore(db))
		ctx := context.Background()
		if _, err := service.Create(ctx, "user-1", "coding"); err != nil {
			t.Fatal(err)
		}
		if _, err := service.Create(ctx, "user-2", "private"); err != nil {
			t.Fatal(err)
		}
		if _, err := service.Create(ctx, "user-1", "writing"); err != nil {
			t.Fatal(err)
		}

		stdout, stderr, err := runSubjectsCLI(t, dsn, "user-1", "list")

		if err != nil {
			t.Fatalf("run CLI: %v: %s", err, stderr)
		}
		if got, want := stdout, "1\tcoding\n3\twriting\n"; got != want {
			t.Fatalf("got output %q, want %q", got, want)
		}

		empty, emptyStderr, err := runSubjectsCLI(t, dsn, "user-3", "list")
		if err != nil {
			t.Fatalf("run CLI for empty list: %v: %s", err, emptyStderr)
		}
		if empty != "" {
			t.Fatalf("got unexpected empty-list output %q", empty)
		}
	})

	t.Run("updates subject", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "user-1")
		service := subject.NewService(data.NewSQLiteSubjectStore(db))
		created, err := service.Create(context.Background(), "user-1", "coding")
		if err != nil {
			t.Fatal(err)
		}

		stdout, stderr, err := runSubjectsCLI(t, dsn, "user-1", "update", "1", "  Go programming  ")

		if err != nil {
			t.Fatalf("run CLI: %v: %s", err, stderr)
		}
		if got, want := stdout, "Updated subject 1:   Go programming  \n"; got != want {
			t.Fatalf("got output %q, want %q", got, want)
		}
		updated, err := service.Get(context.Background(), "user-1", created.SubjectID)
		if err != nil {
			t.Fatal(err)
		}
		if updated.SubjectName != "  Go programming  " {
			t.Fatalf("got updated subject name %q", updated.SubjectName)
		}
	})

	t.Run("prevents cross-user access and mutation", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "user-1", "user-2")
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
			stdout, _, err := runSubjectsCLI(t, dsn, "user-2", args...)
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
		insertUsers(t, db, "user-1")
		ctx := context.Background()
		subjectService := subject.NewService(data.NewSQLiteSubjectStore(db))
		created, err := subjectService.Create(ctx, "user-1", "coding")
		if err != nil {
			t.Fatal(err)
		}
		thoughtService := thought.NewService(data.NewSQLiteThoughtStore(db))
		linked, err := thoughtService.Create(ctx, "user-1", "keep me", &created.SubjectID, time.Time{})
		if err != nil {
			t.Fatal(err)
		}

		stdout, stderr, err := runSubjectsCLI(t, dsn, "user-1", "delete", "1")

		if err != nil {
			t.Fatalf("run CLI: %v: %s", err, stderr)
		}
		if got, want := stdout, "Deleted subject 1\n"; got != want {
			t.Fatalf("got output %q, want %q", got, want)
		}
		if _, err := subjectService.Get(ctx, "user-1", created.SubjectID); !errors.Is(err, data.ErrRecordNotFound) {
			t.Fatalf("got deleted subject error %v, want %v", err, data.ErrRecordNotFound)
		}
		unlinked, err := thoughtService.Get(ctx, "user-1", linked.ThoughtID)
		if err != nil {
			t.Fatal(err)
		}
		if unlinked.SubjectID != nil {
			t.Fatalf("got linked subject ID %#v after deletion", unlinked.SubjectID)
		}
	})

	t.Run("enforces user foreign key", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)

		stdout, _, err := runSubjectsCLI(t, dsn, "missing-user", "create", "learn Go")

		if err == nil {
			t.Fatal("expected foreign key error")
		}
		if stdout != "" {
			t.Fatalf("got unexpected output %q", stdout)
		}
		var count int
		if err := db.QueryRow("SELECT count(*) FROM subjects").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("got %d subjects, want 0", count)
		}
	})
}

func runSubjectsCLI(t *testing.T, dsn, userID string, args ...string) (string, string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	commandArgs := []string{
		"run", "../cmd/thoughts",
		"--db-dsn", dsn,
		"--user-id", userID,
		"subjects",
	}
	command := exec.CommandContext(ctx, "go", append(commandArgs, args...)...)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}
