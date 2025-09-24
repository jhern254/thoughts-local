// server.go 
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
    "github.com/julienschmidt/httprouter" 
)


// TODO: refactor to return *httprouter.Router
func (s *ThoughtServer) routes() *httprouter.Router {
    router := httprouter.New()

    // REST endpoints
    router.HandlerFunc(http.MethodGet, "/subjects/:subject", s.subjectsHandler)
    router.HandlerFunc(http.MethodPost, "/subjects/:subject", s.subjectsHandler)
    router.HandlerFunc(http.MethodGet, "/healthcheck", s.healthcheckHandler)

    return router
}


