package data

import (
	"testing"
    "context"
//    "io"
    "os"
//    "time"
    "github.com/jhern254/go-thoughts/internal/testutils"
)

func TestFileSystemStore(t *testing.T) {
    t.Run("get thoughts from a reader", func(t *testing.T) {
        database, cleanDatabase := CreateTempFile(t, `[
    {
    "UserID": "2",
    "Subjects": [
      {
        "Name": "Art",
        "Thoughts": ["Ye is so talented"]
      },
      {
        "Name": "AI",
        "Thoughts": ["Transformers changed the world!"]
      }
    ]
    }
        ]`)
        defer cleanDatabase()

        store, err := NewFileSystemThoughtStore(database)
        got := store.GetThoughts("2", "AI")
        want := []string{"Transformers changed the world!", }

        testutils.AssertCorrectStruct(t, got, want)
        testutils.AssertNoError(t, err)
    })
    t.Run("store thoughts for existing user", func(t *testing.T) {
        database, cleanDatabase := CreateTempFile(t, `[
    {
      "UserID": "1",
      "Subjects": [
        {
          "Name": "Code",
          "Thoughts": ["I'm learning go!"]
        }
      ]
    }
        ]`)
        defer cleanDatabase()

        store, err := NewFileSystemThoughtStore(database)
        id, err := store.CaptureThought(context.Background(), "1", "AI", "Transformers go brr")
        testutils.AssertNoError(t, err)
        if id <= 0 {
            t.Fatalf("got id %d, want >0", id)
        }
        got := store.GetThoughts("1", "AI")
        want := []string{"Transformers go brr", }

        testutils.AssertCorrectStruct(t, got, want)
        testutils.AssertNoError(t, err)
    })
    t.Run("works with an empty file", func(t *testing.T) {
        database, cleanDatabase := CreateTempFile(t, "")
        defer cleanDatabase()

        _, err := NewFileSystemThoughtStore(database)

        testutils.AssertNoError(t, err)
    })

}


func CreateTempFile(t testing.TB, initialData string) (*os.File, func()) {
    t.Helper()

    tmpfile, err := os.CreateTemp("", "db")

    if err != nil {
        t.Fatalf("could not create temp file %v", err)
    }

    tmpfile.Write([]byte(initialData))

    removeFile := func() {
        tmpfile.Close()
        os.Remove(tmpfile.Name())
    }

    return tmpfile, removeFile

}

