package main

import (
	"strings"
	"testing"
)

func TestFileSystemStore(t *testing.T) {
    t.Run("UserStore from a reader", func(t *testing.T) {
        database := strings.NewReader(`[
{
  "UserID": "1",
  "Subjects": [
    {
      "Name": "Physics",
      "Thought": ["Idk physics"]
    },
    {
      "Name": "Code",
      "Thought": ["I'm learning go!"]
    },
    {
      "Name": "AI",
      "Thought": ["Neural Networks work incredible!"]
    }
  ]
}
]`)

        store := FileSystemThoughtStore{database}

        got := store.GetUserState("1")

        want := UserState{
                    UserID: "1",
                    Subjects: []Subject{
                        {"Physics", []string{"Idk physics"}, },
                        {"Code", []string{"I'm learning go!"}, },
                        {"AI", []string{"Neural Networks work incredible!"}, },
                    },
            }

        assertCorrectStruct(t, got, want)

    })
}


/*
func TestFileSystemStore(t *testing.T) {
    t.Run("UserStore from a reader", func(t *testing.T) {
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

        got := store.GetUserState()

        want := []UserState{
            {
                UserID: "1",
                Subjects: []Subject{
                    {"Physics", []string{"Idk physics"}, },
                    {"Code", []string{"I'm learning go!"}, },
                    {"AI", []string{"Neural Networks work!"}, },
                },
            },
            {
            UserID: "2",
            Subjects: []Subject{
                {"Art", []string{"Ye is so talented"}, },
                {"AI", []string{"Transformers changed the world!"}, },
                },
            },
        }

        assertCorrectStruct(t, got, want)
        
    })
}
*/
