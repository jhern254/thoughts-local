// server_integration_test.go
package main

import (
    "testing"
    "net/http"
    "net/http/httptest"
//    "reflect"
)

func TestPostingThoughtsAndGettingThem(t *testing.T) {
//    store := NewInMemoryThoughtStore()
    database, cleanDatabase := createTempFile(t, "")
    defer cleanDatabase()
    store := &FileSystemThoughtStore{database}
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
}



