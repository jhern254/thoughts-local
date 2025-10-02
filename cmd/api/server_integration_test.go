// server_integration_test.go
package main

import (
    "testing"
    "os"
    "net/http"
    "net/http/httptest"
    "encoding/json"
//    "reflect"
    "github.com/jhern254/go-thoughts/internal/testutils"
    "github.com/jhern254/go-thoughts/internal/data"
    "github.com/rs/zerolog" 
)

// TODO: move(?)
//type subjectEnvelope struct {
//    Subject subjectThoughtResponse `json:"subject"`
//}


func TestPostingThoughtsAndGettingThem(t *testing.T) {
//    store := NewInMemoryThoughtStore()
    database, cleanDatabase := testutils.CreateTempFile(t, `[]`)
    defer cleanDatabase()
    store, err := data.NewFileSystemThoughtStore(database)
    testutils.AssertNoError(t, err)

    server := NewApplication(store, config{}, zerolog.New(os.Stdout))
    subject := "ai"

    server.routes().ServeHTTP(httptest.NewRecorder(), newPostThoughtRequest(subject, "neural networks"))
    server.routes().ServeHTTP(httptest.NewRecorder(), newPostThoughtRequest(subject, "are"))
    server.routes().ServeHTTP(httptest.NewRecorder(), newPostThoughtRequest(subject, "black magic!"))


    t.Run("get thought", func(t *testing.T) {
        response := httptest.NewRecorder()
        server.routes().ServeHTTP(response, newGetThoughtRequest(subject))

        testutils.AssertCorrect(t, response.Code, http.StatusOK)
//        testutils.AssertCorrect(t, response.Body.String(), "neural networks\nare\nblack magic!")

        testutils.AssertContentType(t, response, jsonContentType)

        // decode JSON body
        var env subjectEnvelope
        if err := json.NewDecoder(response.Body).Decode(&env); err != nil {
            t.Fatalf("decode json: %v\nbody:\n%s", err, response.Body.String())
        }
        got := env.Subject

        testutils.AssertCorrect(t, got.SubjectName, "ai")
        testutils.AssertCorrectStruct(t, got.Thoughts, []string{"neural networks", "are", "black magic!"})
    })
}



