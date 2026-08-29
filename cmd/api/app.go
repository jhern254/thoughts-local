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

	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/rs/zerolog"
	//    "github.com/julienschmidt/httprouter"
)

type config struct {
	port int
	env  string
	db   struct {
		dsn          string
		maxOpenConns int
		maxIdleConns int
		maxIdleTime  string
	}
}

// main DI container
type application struct {
	subjectService *subject.Service
	thoughtService *thought.Service
	userFromReq    func(*http.Request) string
	config         config
	logger         zerolog.Logger
}

// ctor
func NewApplication(subjectService *subject.Service, thoughtService *thought.Service, cfg config, logger zerolog.Logger) *application {
	return &application{
		subjectService: subjectService,
		thoughtService: thoughtService,
		userFromReq:    func(*http.Request) string { return "test-user" }, // NOTE: default for now
		config:         cfg,
		logger:         logger,
	}
}
