// server_test.go 
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
    server := NewThoughtServer(store, config{env: "development"}, zerolog.New(os.Stdout))

    t.Run("returns heatlh check info", func(t *testing.T) {
        request :=  httptest.NewRequest(http.MethodGet, "/healthcheck", nil)
        response := httptest.NewRecorder()

        server.routes().ServeHTTP(response, request)

        want := `{"status":"available","environment":"development","version":"0.1.0"}` + "\n"

        testutils.AssertCorrect(t, response.Code, http.StatusOK)
        testutils.AssertCorrect(t, response.Body.String(), want)
    })
}

