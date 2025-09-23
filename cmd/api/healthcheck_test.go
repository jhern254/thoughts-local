// healthcheck_test.go 
package main

import (
    "testing"
    "net/http"
    "net/http/httptest"
//    "fmt"
//    "strings"
    "os"
//    "encoding/json"
//    "io"
    "github.com/jhern254/go-thoughts/internal/data"
    "github.com/jhern254/go-thoughts/internal/testutils"
    "github.com/rs/zerolog" 
)

func TestHealthcheck(t *testing.T) {
    store := data.NewInMemoryThoughtStore()
    server := NewThoughtServer(store, config{}, zerolog.New(os.Stdout))

    t.Run("returns heatlh check info", func(t *testing.T) {
        request :=  httptest.NewRequest(http.MethodGet, "/healthcheck", nil)
        response := httptest.NewRecorder()

        server.ServeHTTP(response, request)

        testutils.AssertCorrect(t, response.Code, http.StatusOK)
    })
}

