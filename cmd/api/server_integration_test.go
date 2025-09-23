// server_integration_test.go
package main

import (
    "testing"
    "os"
    "net/http"
    "net/http/httptest"
//    "reflect"
    "github.com/jhern254/go-thoughts/internal/testutils"
    "github.com/jhern254/go-thoughts/internal/data"
    "github.com/rs/zerolog" 
)

func TestPostingThoughtsAndGettingThem(t *testing.T) {
//    store := NewInMemoryThoughtStore()
    database, cleanDatabase := testutils.CreateTempFile(t, `[]`)
    defer cleanDatabase()
    store, err := data.NewFileSystemThoughtStore(database)
    testutils.AssertNoError(t, err)

    server := NewThoughtServer(store, config{}, zerolog.New(os.Stdout))
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



