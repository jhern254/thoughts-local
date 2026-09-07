package main

import (
	"io"

	"github.com/rs/zerolog"
)

func newSubjectTestApplication(service SubjectService, out io.Writer) *application {
	return &application{
		subjects: service,
		userID:   "user-1",
		out:      out,
		logger:   zerolog.Nop(),
	}
}
