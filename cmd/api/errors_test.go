// errors.go 
package main

import (
//    "fmt"
    "testing"
    "net/http"
    "net/http/httptest"
//    "strings"
//    "os"
    "encoding/json"
//    "io"
    "github.com/jhern254/go-thoughts/internal/testutils"

//    "github.com/rs/zerolog" 
)

func TestNotFoundResponse(t *testing.T) {
    a := &application{ /* stub store, config, zerolog logger */ }

    t.Run("returns heatlh check info", func(t *testing.T) {
        response := httptest.NewRecorder()
        request := httptest.NewRequest(http.MethodGet, "/missing", nil)

        a.notFoundResponse(response, request)

        var body map[string]string
        if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
            t.Fatalf("decode error: %v", err)
        }
        testutils.AssertCorrect(t, response.Code, http.StatusNotFound)
        testutils.AssertCorrect(t, body["error"], "the requested resource could not be found")
        testutils.AssertContentType(t, response, jsonContentType)
    })
}

