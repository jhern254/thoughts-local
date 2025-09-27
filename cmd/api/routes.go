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
func (s *application) routes() *httprouter.Router {
    router := httprouter.New()

    // REST endpoints
    router.HandlerFunc(http.MethodGet, "/subjects/:subject", s.showSubjectHandler)
    router.HandlerFunc(http.MethodGet, "/subjects/:subject/thoughts", s.showSubjectThoughtsHandler)
    router.HandlerFunc(http.MethodPost, "/subjects/:subject/thoughts", s.createSubjectThoughtHandler)
    router.HandlerFunc(http.MethodGet, "/healthcheck", s.healthcheckHandler)

    return router
}


