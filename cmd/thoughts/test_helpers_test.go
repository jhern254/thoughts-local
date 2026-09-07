package main

import (
	"io"

	"github.com/jhern254/go-thoughts/internal/logging"
)

func newSubjectTestApplication(service SubjectService, out io.Writer) *application {
	return &application{
		subjects: service,
		userID:   "user-1",
		out:      out,
		logger:   logging.Nop(),
	}
}
