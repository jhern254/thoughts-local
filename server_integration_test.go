package main

import (
    "testing"
    "net/http"
    "net/http/httptest"
//    "reflect"
)


func TestPostingThoughtsAndGettingThem(t *testing.T) {
    store := NewInMemoryThoughtStore()
    server := NewThoughtServer(store)
    subject := "ai"

    server.ServeHTTP(httptest.NewRecorder(), newPostThoughtRequest(subject, "neural networks"))
    server.ServeHTTP(httptest.NewRecorder(), newPostThoughtRequest(subject, "are"))
    server.ServeHTTP(httptest.NewRecorder(), newPostThoughtRequest(subject, "black magic!"))


    t.Run("get thought", func(t *testing.T) {
        response := httptest.NewRecorder()
        server.ServeHTTP(response, newGetThoughtRequest(subject))

        assertCorrect(t, response.Code, http.StatusOK)
        assertCorrect(t, response.Body.String(), "neural networks\nare\nblack magic!")
    })
    t.Run("get userState", func(t *testing.T) {
        response := httptest.NewRecorder()
        server.ServeHTTP(response, newUserStateRequest("1"))

        got := getUserStateFromResponse(t, response.Body)
        want := UserState{
            UserID: "1",
            Subjects: []Subject{
                {
                    Name:    "ai",
                    Thought: []string{
                        "neural networks",
                        "are",
                        "black magic!",
                    },
                },
            },
        }

        assertCorrectStruct(t, got, want)
    })
}



