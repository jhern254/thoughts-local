package data

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func newTestFileSystemStore(t *testing.T) (*FileSystemStore, *os.File) {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "store-*.json")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { file.Close() })
	store, err := NewFileSystemStore(file)
	if err != nil {
		t.Fatal(err)
	}
	return store, file
}

func TestFileSystemStore_CreateSubject(t *testing.T) {
	t.Run("creates and persists subject", func(t *testing.T) {
		store, file := newTestFileSystemStore(t)
		now := time.Now().UTC()

		created, err := store.CreateSubject(context.Background(), &Subject{UserID: "test-user", SubjectName: "coding", CreatedAt: now, UpdatedAt: now})

		if err != nil {
			t.Fatal(err)
		}
		if created.SubjectID != 1 || created.SubjectName != "coding" {
			t.Fatalf("got subject %#v", created)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		reopenedFile, err := os.OpenFile(file.Name(), os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer reopenedFile.Close()
		reopened, err := NewFileSystemStore(reopenedFile)
		if err != nil {
			t.Fatal(err)
		}
		got, err := reopened.GetSubject(context.Background(), "test-user", created.SubjectID)
		if err != nil {
			t.Fatal(err)
		}
		if got.SubjectName != "coding" {
			t.Fatalf("got subject %#v", got)
		}
	})

	t.Run("rejects duplicate subject", func(t *testing.T) {
		store, _ := newTestFileSystemStore(t)
		item := &Subject{UserID: "test-user", SubjectName: "coding"}
		if _, err := store.CreateSubject(context.Background(), item); err != nil {
			t.Fatal(err)
		}

		_, err := store.CreateSubject(context.Background(), item)

		if !errors.Is(err, ErrDuplicateRecord) {
			t.Fatalf("got %v, want %v", err, ErrDuplicateRecord)
		}
	})
}

func TestFileSystemStore_GetSubject(t *testing.T) {
	t.Run("returns subject", func(t *testing.T) {
		store, _ := newTestFileSystemStore(t)
		created, err := store.CreateSubject(context.Background(), &Subject{UserID: "test-user", SubjectName: "coding"})
		if err != nil {
			t.Fatal(err)
		}

		got, err := store.GetSubject(context.Background(), "test-user", created.SubjectID)

		if err != nil {
			t.Fatal(err)
		}
		if got.SubjectID != created.SubjectID || got.SubjectName != "coding" {
			t.Fatalf("got subject %#v", got)
		}
	})

	t.Run("returns not found", func(t *testing.T) {
		store, _ := newTestFileSystemStore(t)

		_, err := store.GetSubject(context.Background(), "test-user", 99)

		if !errors.Is(err, ErrRecordNotFound) {
			t.Fatalf("got %v, want %v", err, ErrRecordNotFound)
		}
	})

	t.Run("does not return another users subject", func(t *testing.T) {
		store, _ := newTestFileSystemStore(t)
		created, err := store.CreateSubject(context.Background(), &Subject{UserID: "another-user", SubjectName: "coding"})
		if err != nil {
			t.Fatal(err)
		}

		_, err = store.GetSubject(context.Background(), "test-user", created.SubjectID)

		if !errors.Is(err, ErrRecordNotFound) {
			t.Fatalf("got %v, want %v", err, ErrRecordNotFound)
		}
	})
}

func TestFileSystemStore_CreateThought(t *testing.T) {
	t.Run("creates and persists thought", func(t *testing.T) {
		store, file := newTestFileSystemStore(t)
		now := time.Now().UTC()
		subject, err := store.CreateSubject(context.Background(), &Subject{UserID: "test-user", SubjectName: "coding", CreatedAt: now, UpdatedAt: now})
		if err != nil {
			t.Fatal(err)
		}

		created, err := store.CreateThought(context.Background(), &Thought{UserID: "test-user", SubjectID: &subject.SubjectID, Thought: "learn Go", Version: 1, ObservedAt: now, CreatedAt: now, UpdatedAt: now})

		if err != nil {
			t.Fatal(err)
		}
		if created.ThoughtID != 1 {
			t.Fatalf("got thought ID %d", created.ThoughtID)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		reopenedFile, err := os.OpenFile(file.Name(), os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer reopenedFile.Close()
		reopened, err := NewFileSystemStore(reopenedFile)
		if err != nil {
			t.Fatal(err)
		}
		got, err := reopened.GetThought(context.Background(), "test-user", created.ThoughtID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Thought != "learn Go" || got.SubjectID == nil || *got.SubjectID != subject.SubjectID {
			t.Fatalf("got thought %#v", got)
		}
	})

	t.Run("creates thought without subject", func(t *testing.T) {
		store, _ := newTestFileSystemStore(t)

		created, err := store.CreateThought(context.Background(), &Thought{UserID: "test-user", Thought: "unassigned"})

		if err != nil {
			t.Fatal(err)
		}
		if created.ThoughtID != 1 || created.SubjectID != nil {
			t.Fatalf("got thought %#v", created)
		}
	})

	t.Run("rejects missing subject", func(t *testing.T) {
		store, _ := newTestFileSystemStore(t)
		subjectID := int64(99)

		_, err := store.CreateThought(context.Background(), &Thought{UserID: "test-user", SubjectID: &subjectID, Thought: "orphan"})

		if !errors.Is(err, ErrRecordNotFound) {
			t.Fatalf("got %v, want %v", err, ErrRecordNotFound)
		}
	})

	t.Run("rejects another users subject", func(t *testing.T) {
		store, _ := newTestFileSystemStore(t)
		subject, err := store.CreateSubject(context.Background(), &Subject{UserID: "another-user", SubjectName: "coding"})
		if err != nil {
			t.Fatal(err)
		}

		_, err = store.CreateThought(context.Background(), &Thought{UserID: "test-user", SubjectID: &subject.SubjectID, Thought: "orphan"})

		if !errors.Is(err, ErrRecordNotFound) {
			t.Fatalf("got %v, want %v", err, ErrRecordNotFound)
		}
	})
}

func TestFileSystemStore_GetThought(t *testing.T) {
	t.Run("returns thought", func(t *testing.T) {
		store, _ := newTestFileSystemStore(t)
		created, err := store.CreateThought(context.Background(), &Thought{UserID: "test-user", Thought: "learn Go"})
		if err != nil {
			t.Fatal(err)
		}

		got, err := store.GetThought(context.Background(), "test-user", created.ThoughtID)

		if err != nil {
			t.Fatal(err)
		}
		if got.ThoughtID != created.ThoughtID || got.Thought != "learn Go" {
			t.Fatalf("got thought %#v", got)
		}
	})

	t.Run("returns not found", func(t *testing.T) {
		store, _ := newTestFileSystemStore(t)

		_, err := store.GetThought(context.Background(), "test-user", 99)

		if !errors.Is(err, ErrRecordNotFound) {
			t.Fatalf("got %v, want %v", err, ErrRecordNotFound)
		}
	})

	t.Run("does not return another users thought", func(t *testing.T) {
		store, _ := newTestFileSystemStore(t)
		created, err := store.CreateThought(context.Background(), &Thought{UserID: "another-user", Thought: "private"})
		if err != nil {
			t.Fatal(err)
		}

		_, err = store.GetThought(context.Background(), "test-user", created.ThoughtID)

		if !errors.Is(err, ErrRecordNotFound) {
			t.Fatalf("got %v, want %v", err, ErrRecordNotFound)
		}
	})
}

func TestNewFileSystemStore(t *testing.T) {
	t.Run("opens empty file", func(t *testing.T) {
		_, _ = newTestFileSystemStore(t)
	})

	t.Run("rejects legacy nested JSON", func(t *testing.T) {
		file, err := os.CreateTemp(t.TempDir(), "store-*.json")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if _, err := file.WriteString(`[{"UserID":"test-user","Subjects":[]}]`); err != nil {
			t.Fatal(err)
		}

		_, err = NewFileSystemStore(file)

		if err == nil {
			t.Fatal("expected legacy JSON error")
		}
	})
}
