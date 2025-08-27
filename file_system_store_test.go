package main

import (
	"strings"
	"testing"
)

func TestFileSystemStore(t *testing.T) {
    t.Run("UserStates from a reader", func(t *testing.T) {
        database := strings.NewReader(`[
{
  "UserID": "1",
  "Subjects": [
    {
      "Name": "Physics",
      "Thoughts": ["Idk physics"]
    },
    {
      "Name": "Code",
      "Thoughts": ["I'm learning go!"]
    },
    {
      "Name": "AI",
      "Thoughts": ["Neural Networks work incredible!"]
    }
  ]
},
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

        store := FileSystemThoughtStore{database}

        got := store.GetAllUserStates()
        want := []UserState{
            {
                UserID: "1",
                Subjects: []Subject{
                    {"Physics", []string{"Idk physics"}},
                    {"Code", []string{"I'm learning go!"}},
                    {"AI", []string{"Neural Networks work incredible!"}},
                },
            },
            {
                UserID: "2",
                Subjects: []Subject{
                    {"Art", []string{"Ye is so talented"}},
                    {"AI", []string{"Transformers changed the world!"}},
                },
            },
        }

        assertCorrectStruct(t, got, want)

        // read twice
        got = store.GetAllUserStates()
        assertCorrectStruct(t, got, want)

    })
    t.Run("Get Thoughts from a reader", func(t *testing.T) {
        database := strings.NewReader(`[
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

        store := FileSystemThoughtStore{database}
        got := store.GetThoughts("AI")
        want := []string{"Transformers changed the world!", }

        assertCorrectStruct(t, got, want)
    })


}

