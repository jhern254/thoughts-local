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


func (s *ThoughtServer) healthcheckHandler(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
}

