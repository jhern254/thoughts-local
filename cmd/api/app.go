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
	"github.com/jhern254/go-thoughts/internal/data"
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
	subjects       data.SubjectStore
	thoughts       data.ThoughtStore
	subjectService *subject.Service
	thoughtService *thought.Service
	userFromReq    func(*http.Request) string
	config         config
	logger         zerolog.Logger
}

// NOTE: pattern is server fns here, store interface done on test, main

// ctor
// NOTE: store interface already reference value, impl needs pointer
func NewApplication(subjects data.SubjectStore, thoughts data.ThoughtStore, cfg config, logger zerolog.Logger) *application {
	return &application{
		subjects:       subjects,
		thoughts:       thoughts,
		subjectService: subject.NewService(subjects),
		thoughtService: thought.NewService(thoughts),
		userFromReq:    func(*http.Request) string { return "test-user" }, // NOTE: default for now
		config:         cfg,
		logger:         logger,
	}
}
