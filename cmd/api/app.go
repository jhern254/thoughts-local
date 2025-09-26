// app.go 
// di container
package main

import (
//    "fmt"
//    "os"
    "net/http"
//    "errors"
//    "strings"
//    "io"
//    "encoding/json"

    "github.com/rs/zerolog" 
//    "github.com/julienschmidt/httprouter" 
)

type config struct {
    port int
    env string
}

// main DI container
type application struct {
    store       ThoughtStore
    userFromReq func(*http.Request) string
    config      config
    logger      zerolog.Logger     
}
// NOTE: pattern is server fns here, store interface done on test, main

// ctor
// NOTE: store interface already reference value, impl needs pointer
func NewApplication(store ThoughtStore, cfg config, logger zerolog.Logger) *application {
    return &application{
        store:      store,
        userFromReq: func(*http.Request) string { return "test-user" }, // NOTE: default for now
        config: cfg,
        logger: logger,
    }
}

