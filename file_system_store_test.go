package main

import (
	"testing"
//    "io"
    "os"
)

func TestFileSystemStore(t *testing.T) {
    t.Run("get thoughts from a reader", func(t *testing.T) {
        database, cleanDatabase := createTempFile(t, `[
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

        assertCorrectStruct(t, got, want)
        assertNoError(t, err)
    })
    t.Run("store thoughts for existing user", func(t *testing.T) {
        database, cleanDatabase := createTempFile(t, `[
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
        store.CaptureThought("1", "AI", "Transformers go brr")
        got := store.GetThoughts("1", "AI")
        want := []string{"Transformers go brr", }

        assertCorrectStruct(t, got, want)
        assertNoError(t, err)
    })
    t.Run("works with an empty file", func(t *testing.T) {
        database, cleanDatabase := createTempFile(t, "")
        defer cleanDatabase()

        _, err := NewFileSystemThoughtStore(database)

        assertNoError(t, err)
    })

}


func createTempFile(t testing.TB, initialData string) (*os.File, func()) {
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

