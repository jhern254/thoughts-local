package main

import (
	"testing"
    "io"
    "os"
)

func TestFileSystemStore(t *testing.T) {
//    t.Run("UserStates from a reader", func(t *testing.T) {
//        database, cleanDatabase := createTempFile(t, `[
//{
//  "UserID": "1",
//  "Subjects": [
//    {
//      "Name": "Physics",
//      "Thoughts": ["Idk physics"]
//    },
//    {
//      "Name": "Code",
//      "Thoughts": ["I'm learning go!"]
//    },
//    {
//      "Name": "AI",
//      "Thoughts": ["Neural Networks work incredible!"]
//    }
//  ]
//},
//{
//"UserID": "2",
//"Subjects": [
//  {
//    "Name": "Art",
//    "Thoughts": ["Ye is so talented"]
//  },
//  {
//    "Name": "AI",
//    "Thoughts": ["Transformers changed the world!"]
//  }
//]
//}
//]`)
//        defer cleanDatabase()
//
//        store := FileSystemThoughtStore{database}
//
//        got := store.GetAllUserStates()
//        want := []UserState{
//            {
//                UserID: "1",
//                Subjects: []Subject{
//                    {"Physics", []string{"Idk physics"}},
//                    {"Code", []string{"I'm learning go!"}},
//                    {"AI", []string{"Neural Networks work incredible!"}},
//                },
//            },
//            {
//                UserID: "2",
//                Subjects: []Subject{
//                    {"Art", []string{"Ye is so talented"}},
//                    {"AI", []string{"Transformers changed the world!"}},
//                },
//            },
//        }
//
//        assertCorrectStruct(t, got, want)
//
//        // read twice
//        got = store.GetAllUserStates()
//        assertCorrectStruct(t, got, want)
//
//    })
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

        store, _ := NewFileSystemThoughtStore(database)
        got := store.GetThoughts("2", "AI")
        want := []string{"Transformers changed the world!", }

        assertCorrectStruct(t, got, want)
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

        store, _ := NewFileSystemThoughtStore(database)
        store.CaptureThought("1", "AI", "Transformers go brr")
        got := store.GetThoughts("1", "AI")
        want := []string{"Transformers go brr", }

        assertCorrectStruct(t, got, want)
    })

}


func createTempFile(t testing.TB, initialData string) (io.ReadWriteSeeker, func()) {
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

