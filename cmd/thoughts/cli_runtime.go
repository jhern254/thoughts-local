package main

import (
	"context"
	"io"

	appcore "github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/rs/zerolog"
)

type cliRuntime interface {
	LocalUser() *data.User
	Subjects() *subject.Service
	Close() error
}

type application struct {
	runtime  cliRuntime
	subjects SubjectService
	userID   string
	out      io.Writer
	errOut   io.Writer
	logger   zerolog.Logger

	openRuntime func(context.Context, string) (cliRuntime, error)
}

func newApplication(out, errOut io.Writer, logger zerolog.Logger) *application {
	return &application{
		out:    out,
		errOut: errOut,
		logger: logger,
		openRuntime: func(ctx context.Context, dsn string) (cliRuntime, error) {
			return appcore.Open(ctx, dsn)
		},
	}
}

func (app *application) open(ctx context.Context, dsn string) error {
	runtime, err := app.openRuntime(ctx, dsn)
	if err != nil {
		return err
	}

	app.runtime = runtime
	app.subjects = runtime.Subjects()
	app.userID = runtime.LocalUser().UserID
	return nil
}

func (app *application) close() error {
	if app.runtime == nil {
		return nil
	}

	err := app.runtime.Close()
	app.runtime = nil
	app.subjects = nil
	app.userID = ""
	return err
}
