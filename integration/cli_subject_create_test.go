//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
)

func TestSubjectCreateCLI_SQLite(t *testing.T) {
	t.Run("creates subject", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		insertUsers(t, db, "user-1")
		var stdout, stderr bytes.Buffer
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		command := exec.CommandContext(
			ctx,
			"go", "run", "../cmd/thoughts",
			"--db-dsn", dsn,
			"--user-id", "user-1",
			"subjects", "create", "  learn Go  ",
		)
		command.Stdout = &stdout
		command.Stderr = &stderr

		if err := command.Run(); err != nil {
			t.Fatalf("run CLI: %v: %s", err, stderr.String())
		}
		if got, want := stdout.String(), "Created subject 1:   learn Go  \n"; got != want {
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

	t.Run("enforces user foreign key", func(t *testing.T) {
		db, dsn := openMigratedSQLite(t)
		var stdout, stderr bytes.Buffer
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		command := exec.CommandContext(
			ctx,
			"go", "run", "../cmd/thoughts",
			"--db-dsn", dsn,
			"--user-id", "missing-user",
			"subjects", "create", "learn Go",
		)
		command.Stdout = &stdout
		command.Stderr = &stderr

		if err := command.Run(); err == nil {
			t.Fatal("expected foreign key error")
		}
		if stdout.Len() != 0 {
			t.Fatalf("got unexpected output %q", stdout.String())
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
