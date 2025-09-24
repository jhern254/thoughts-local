// healthcheck.go 
package main

import (
//    "fmt"
//    "os"
    "net/http"
//    "errors"
//    "strings"
//    "io"
    "encoding/json"

//    "github.com/rs/zerolog" 
)

// NOTE: exported vars for json encoding
type healthcheckResponse struct {
    Status      string `json:"status"`
    Environment string `json:"environment"`
    Version     string `json:"version"`
}

func (s *ThoughtServer) healthcheckHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("content-Type", jsonContentType)
    w.WriteHeader(http.StatusOK)

    resp := healthcheckResponse{
        Status:      "available",
        Environment: s.config.env,
        Version:     version,
    }

    json.NewEncoder(w).Encode(resp)
}

