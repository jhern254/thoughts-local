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
    "github.com/jhern254/go-thoughts/internal/data"
)

type config struct {
    port int
    env string
    db struct {
        dsn string
        maxOpenConns int
        maxIdleConns int
        maxIdleTime string
    }
}

// main DI container
type application struct {
    store       data.Store
    userFromReq func(*http.Request) string
    config      config
    logger      zerolog.Logger     
}
// NOTE: pattern is server fns here, store interface done on test, main

// ctor
// NOTE: store interface already reference value, impl needs pointer
func NewApplication(store data.Store, cfg config, logger zerolog.Logger) *application {
    return &application{
        store:      store,
        userFromReq: func(*http.Request) string { return "test-user" }, // NOTE: default for now
        config: cfg,
        logger: logger,
    }
}

