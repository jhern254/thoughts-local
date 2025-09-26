// healthcheck.go 
package main

import (
//    "fmt"
//    "os"
    "net/http"
//    "errors"
//    "strings"
//    "io"
//    "encoding/json"

//    "github.com/rs/zerolog" 
)

// NOTE: exported vars for json encoding
type healthcheckResponse struct {
    Status      string `json:"status"`
    Environment string `json:"environment"`
    Version     string `json:"version"`
}

func (s *application) healthcheckHandler(w http.ResponseWriter, r *http.Request) {
    resp := healthcheckResponse{
        Status:      "available",
        Environment: s.config.env,
        Version:     version,
    }

//    json.NewEncoder(w).Encode(resp)
    err := s.writeJSON(w, http.StatusOK, resp, nil)
    if err != nil {
        s.logger.Println(err)
        http.Error(w, "The server encountered a problem and could not proccess your request", http.StatusInternalServerError)
        return
    }
}

