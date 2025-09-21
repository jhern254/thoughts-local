// server_integration_test.go
package main

import (
    "testing"
    "net/http"
    "net/http/httptest"
//    "reflect"
    "github.com/jhern254/go-thoughts/internal/testutils"
    "github.com/jhern254/go-thoughts/internal/data"
)

func TestPostingThoughtsAndGettingThem(t *testing.T) {
//    store := NewInMemoryThoughtStore()
    database, cleanDatabase := testutils.CreateTempFile(t, `[]`)
    defer cleanDatabase()
    store, err := data.NewFileSystemThoughtStore(database)
    testutils.AssertNoError(t, err)

    server := NewThoughtServer(store)
    subject := "ai"

    server.ServeHTTP(httptest.NewRecorder(), newPostThoughtRequest(subject, "neural networks"))
    server.ServeHTTP(httptest.NewRecorder(), newPostThoughtRequest(subject, "are"))
    server.ServeHTTP(httptest.NewRecorder(), newPostThoughtRequest(subject, "black magic!"))


    t.Run("get thought", func(t *testing.T) {
        response := httptest.NewRecorder()
        server.ServeHTTP(response, newGetThoughtRequest(subject))

        testutils.AssertCorrect(t, response.Code, http.StatusOK)
        testutils.AssertCorrect(t, response.Body.String(), "neural networks\nare\nblack magic!")
    })
}



