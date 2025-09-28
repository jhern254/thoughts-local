// server_test.go 
package main

import (
    "testing"
    "net/http"
    "net/http/httptest"
//    "fmt"
//    "strings"
    "os"
    "encoding/json"
//    "io"
    "github.com/jhern254/go-thoughts/internal/data"
    "github.com/jhern254/go-thoughts/internal/testutils"
    "github.com/rs/zerolog" 
)

func TestHealthcheck(t *testing.T) {
    store := data.NewInMemoryThoughtStore()
    server := NewApplication(store, config{env: "development"}, zerolog.New(os.Stdout))

    t.Run("returns heatlh check info", func(t *testing.T) {
        request :=  httptest.NewRequest(http.MethodGet, "/healthcheck", nil)
        response := httptest.NewRecorder()

        server.routes().ServeHTTP(response, request)

        testutils.AssertCorrect(t, response.Code, http.StatusOK)

        var got struct {
            Status     string `json:"status"`
            SystemInfo struct {
                Environment string `json:"environment"`
                Version     string `json:"version"`
            } `json:"system_info"`
        }

        if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
            t.Fatalf("decode error: %v", err)
        }
        testutils.AssertCorrect(t, got.Status, "available")
        testutils.AssertCorrect(t, got.SystemInfo.Environment, "development")
        testutils.AssertCorrect(t, got.SystemInfo.Version, "0.1.0")
    })
}

