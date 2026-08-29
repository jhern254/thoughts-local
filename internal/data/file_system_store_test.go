package data

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestFileSystemStore_CreateThought(t *testing.T) {
	t.Run("creates and persists thought separately from subject", func(t *testing.T) {
		file, err := os.CreateTemp(t.TempDir(), "store-*.json")
		if err != nil {
			t.Fatal(err)
		}
		store, err := NewFileSystemStore(file)
		if err != nil {
			t.Fatal(err)
		}
		now := time.Now().UTC()
		subject, err := store.CreateSubject(context.Background(), &Subject{UserID: "test-user", SubjectName: "coding", CreatedAt: now, UpdatedAt: now})
		if err != nil {
			t.Fatal(err)
		}
		item, err := store.CreateThought(context.Background(), &Thought{UserID: "test-user", SubjectID: &subject.SubjectID, Thought: "learn Go", Version: 1, ObservedAt: now, CreatedAt: now, UpdatedAt: now})
		if err != nil {
			t.Fatal(err)
		}
		if subject.SubjectID == 0 || item.ThoughtID == 0 {
			t.Fatal("expected generated IDs")
		}
		if _, err := store.GetSubject(context.Background(), "test-user", subject.SubjectID); err != nil {
			t.Fatal(err)
		}
		got, err := store.GetThought(context.Background(), "test-user", item.ThoughtID)
		if err != nil {
			t.Fatal(err)
		}
		if got.SubjectID == nil || *got.SubjectID != subject.SubjectID {
			t.Fatalf("got subject ID %#v", got.SubjectID)
		}
		file.Close()
		file, err = os.OpenFile(file.Name(), os.O_RDWR, 0)
		if err != nil {
			t.Fatal(err)
		}
		reopened, err := NewFileSystemStore(file)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := reopened.GetThought(context.Background(), "test-user", item.ThoughtID); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("rejects missing subject", func(t *testing.T) {
		file, err := os.CreateTemp(t.TempDir(), "store-*.json")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		store, err := NewFileSystemStore(file)
		if err != nil {
			t.Fatal(err)
		}
		id := int64(1)
		now := time.Now().UTC()
		if _, err := store.CreateThought(context.Background(), &Thought{UserID: "test-user", SubjectID: &id, Thought: "orphan", Version: 1, ObservedAt: now, CreatedAt: now, UpdatedAt: now}); err != ErrRecordNotFound {
			t.Fatalf("got %v", err)
		}
	})
}

func TestNewFileSystemStore(t *testing.T) {
	t.Run("rejects legacy nested JSON", func(t *testing.T) {
		file, err := os.CreateTemp(t.TempDir(), "store-*.json")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if _, err := file.WriteString(`[{"UserID":"test-user","Subjects":[]}]`); err != nil {
			t.Fatal(err)
		}
		if _, err := NewFileSystemStore(file); err == nil {
			t.Fatal("expected legacy JSON error")
		}
	})
}
