package main

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"strings"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/user"
	_ "modernc.org/sqlite"
)

type application struct {
	db       *sql.DB
	subjects SubjectService
	userID   string
	out      io.Writer
	errOut   io.Writer

	openDatabase func(context.Context, string) (*sql.DB, error)
}

func newApplication(out, errOut io.Writer) *application {
	return &application{
		out:          out,
		errOut:       errOut,
		openDatabase: openSQLite,
	}
}

func (app *application) open(ctx context.Context, dsn string) error {
	db, err := app.openDatabase(ctx, dsn)
	if err != nil {
		return err
	}
	localUser, err := user.NewService(data.NewSQLiteUserStore(db)).EnsureLocalUser(ctx)
	if err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return errors.Join(err, closeErr)
		}
		return err
	}

	app.db = db
	app.subjects = subject.NewService(data.NewSQLiteSubjectStore(db))
	app.userID = localUser.UserID
	return nil
}

func (app *application) close() error {
	if app.db == nil {
		return nil
	}

	err := app.db.Close()
	app.db = nil
	app.subjects = nil
	app.userID = ""
	return err
}

func openSQLite(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", sqliteDSNWithForeignKeys(dsn))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if pingErr := db.PingContext(ctx); pingErr != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, errors.Join(pingErr, closeErr)
		}
		return nil, pingErr
	}
	return db, nil
}

func sqliteDSNWithForeignKeys(dsn string) string {
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	return dsn + separator + "_pragma=foreign_keys(1)"
}
