package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/rs/zerolog"
	_ "modernc.org/sqlite"
)

const (
	version         = "0.1.0"
	jsonContentType = "application/json"
)

func main() {
	cfg, err := parseConfig(os.Args[1:], os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Invalid command arguments. Use -help for usage.")
		os.Exit(2)
	}

	// configure package zerolog
	output := zerolog.ConsoleWriter{
		Out:        os.Stdout, // for stdout
		TimeFormat: "2006-01-02 15:04:05",
	}

	logger := zerolog.New(output).
		With().
		Timestamp().
		Caller(). // adds file and line number
		Logger().
		Level(zerolog.InfoLevel) // set log level

	// set up db
	db, err := openDB(cfg)
	if err != nil {
		logger.Fatal().Msg("Could not open the application database.")
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error().Msg("Could not close the application database.")
		}
	}()
	logger.Info().Msg("database connection pool established")

	// set up server
	subjectService := subject.NewService(data.NewSQLiteSubjectStore(db))
	thoughtService := thought.NewService(data.NewSQLiteThoughtStore(db))
	app := NewApplication(subjectService, thoughtService, cfg, logger)

	addr := fmt.Sprintf(":%d", cfg.port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      app.routes(),
		ErrorLog:     log.New(serverDiagnosticWriter{logger: logger}, "", 0),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// start server
	app.logger.Info().Str("addr", addr).Msg("starting server")
	if err := srv.ListenAndServe(); err != nil {
		app.logger.Fatal().Msg("Could not run the HTTP server.")
	}
	app.logger.Debug().Msg("Program ended.")

}

func openDB(cfg config) (*sql.DB, error) {
	db, err := sql.Open("sqlite", sqliteDSNWithForeignKeys(cfg.db.dsn))
	if err != nil {
		return nil, err
	}

	// set db config
	db.SetMaxOpenConns(cfg.db.maxOpenConns)
	db.SetMaxIdleConns(cfg.db.maxIdleConns)

	idle, err := time.ParseDuration(cfg.db.maxIdleTime)
	if err != nil {
		return nil, err
	}
	db.SetConnMaxIdleTime(idle)

	// create context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// establish conn
	if err := db.PingContext(ctx); err != nil {
		return nil, err
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

// Keep flag-parser errors and environment-derived defaults out of usage output.
func parseConfig(args []string, helpOut io.Writer) (config, error) {
	flags := flag.NewFlagSet("thoughts-api", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.Usage = func() {
		fmt.Fprintln(helpOut, "Usage: thoughts-api [options]")
		flags.SetOutput(helpOut)
		flags.PrintDefaults()
		flags.SetOutput(io.Discard)
	}
	var cfg config

	flags.IntVar(&cfg.port, "port", 7777, "API server port")
	flags.StringVar(&cfg.env, "env", "development", "Environment (development|staging|production)")
	flags.StringVar(&cfg.db.dsn, "db-dsn", "", "SQLite DSN (e.g. file:data/thoughts_dev.db)")
	flags.IntVar(&cfg.db.maxOpenConns, "db-max-open-conns", 4, "SQLite max open connections")
	flags.IntVar(&cfg.db.maxIdleConns, "db-max-idle-conns", 4, "SQLite max idle connections")
	flags.StringVar(&cfg.db.maxIdleTime, "db-max-idle-time", "15m", "SQLite max connection idle time")
	if err := flags.Parse(args); err != nil {
		return cfg, err
	}
	dsnSet := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "db-dsn" {
			dsnSet = true
		}
	})
	if !dsnSet {
		cfg.db.dsn = os.Getenv("THOUGHTS_DB_DSN")
	}
	return cfg, nil
}
