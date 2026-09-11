package application

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/event"
	"github.com/jhern254/go-thoughts/internal/metrics"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/jhern254/go-thoughts/internal/timeline"
	"github.com/jhern254/go-thoughts/internal/user"
	_ "modernc.org/sqlite"
)

const DefaultSQLiteDSN = "file:data/thoughts.db"

type Runtime struct {
	db        *sql.DB
	localUser *data.User
	subjects  *subject.Service
	thoughts  *thought.Service
	metrics   *metrics.Service
	events    *event.Service
	timeline  *timeline.Service
}

func Open(ctx context.Context, dsn string) (*Runtime, error) {
	return open(ctx, dsn, openSQLite, ensureLocalUser)
}

func open(
	ctx context.Context,
	dsn string,
	openDatabase func(context.Context, string) (*sql.DB, error),
	bootstrapLocalUser func(context.Context, *sql.DB) (*data.User, error),
) (*Runtime, error) {
	db, err := openDatabase(ctx, dsn)
	if err != nil {
		return nil, data.TranslateSQLiteError(err)
	}
	localUser, err := bootstrapLocalUser(ctx, db)
	if err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, errors.Join(data.TranslateSQLiteError(err), data.TranslateSQLiteError(closeErr))
		}
		return nil, data.TranslateSQLiteError(err)
	}

	events := event.NewService(data.NewSQLiteEventStore(db))
	thoughtStore := data.NewSQLiteThoughtStore(db)
	return &Runtime{
		db:        db,
		localUser: localUser,
		subjects:  subject.NewService(data.NewSQLiteSubjectStore(db)),
		thoughts:  thought.NewService(thoughtStore),
		metrics:   metrics.NewService(data.NewSQLiteMetricsStore(db)),
		events:    events,
		timeline:  timeline.NewService(events, thoughtStore),
	}, nil
}

func (runtime *Runtime) LocalUser() *data.User {
	return runtime.localUser
}

func (runtime *Runtime) Subjects() *subject.Service {
	return runtime.subjects
}

func (runtime *Runtime) Thoughts() *thought.Service  { return runtime.thoughts }
func (runtime *Runtime) Metrics() *metrics.Service   { return runtime.metrics }
func (runtime *Runtime) Events() *event.Service      { return runtime.events }
func (runtime *Runtime) Timeline() *timeline.Service { return runtime.timeline }

func ensureLocalUser(ctx context.Context, db *sql.DB) (*data.User, error) {
	return user.NewService(data.NewSQLiteUserStore(db)).EnsureLocalUser(ctx)
}

func (runtime *Runtime) Close() error {
	if runtime.db == nil {
		return nil
	}

	err := runtime.db.Close()
	runtime.db = nil
	runtime.localUser = nil
	runtime.subjects = nil
	runtime.thoughts = nil
	runtime.metrics = nil
	runtime.events = nil
	runtime.timeline = nil
	return data.TranslateSQLiteError(err)
}

func openSQLite(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", sqliteDSNWithForeignKeys(dsn))
	if err != nil {
		return nil, data.TranslateSQLiteError(err)
	}
	db.SetMaxOpenConns(1)
	if pingErr := db.PingContext(ctx); pingErr != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, errors.Join(data.TranslateSQLiteError(pingErr), data.TranslateSQLiteError(closeErr))
		}
		return nil, data.TranslateSQLiteError(pingErr)
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
