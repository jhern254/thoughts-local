package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/rs/zerolog"
	_ "modernc.org/sqlite"
)

const (
	dbFileName      = "thoughts.db.json"
	version         = "0.1.0"
	jsonContentType = "application/json"
)

func main() {
	var cfg config

	flag.IntVar(&cfg.port, "port", 7777, "API server port")
	flag.StringVar(&cfg.env, "env", "development", "Environment (development|staging|production)")
	flag.StringVar(&cfg.db.dsn, "db-dsn", os.Getenv("THOUGHTS_DB_DSN"), "SQLite DSN (e.g. file:data/thoughts_dev.db)")
	flag.IntVar(&cfg.db.maxOpenConns, "db-max-open-conns", 4, "SQLite max open connections")
	flag.IntVar(&cfg.db.maxIdleConns, "db-max-idle-conns", 4, "SQLite max idle connections")
	flag.StringVar(&cfg.db.maxIdleTime, "db-max-idle-time", "15m", "SQLite max connection idle time")
	flag.Parse()

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
		logger.Fatal().Err(err).Msg("failed to open SQLite database")
	}
	defer db.Close()
	logger.Info().Str("dsn", cfg.db.dsn).Msg("database connection pool established")

	// NOTE: temp
	// ---------- use existing JSON file store (temporary) ----------
	f, err := os.OpenFile(dbFileName, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		logger.Fatal().Err(err).Str("dbfile", dbFileName).Msg("problem opening file store")
	}

	store, err := data.NewFileSystemStore(f)
	if err != nil {
		logger.Fatal().Err(err).Msg("problem creating file system thought store")
	}

	// initialize store
	// TODO: write fn
	//    store := data.NewSQLiteStore(db)

	// set up server
	subjectService := subject.NewService(store)
	thoughtService := thought.NewService(store)
	app := NewApplication(subjectService, thoughtService, cfg, logger)

	addr := fmt.Sprintf(":%d", cfg.port)
	srv := &http.Server{
		Addr:         addr,
		Handler:      app.routes(),
		IdleTimeout:  time.Minute,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// start server
	app.logger.Info().Str("addr", addr).Msg("starting server")
	if err := srv.ListenAndServe(); err != nil {
		app.logger.Fatal().Err(err).Str("addr", addr).Msg("ListenAndServe failed")
	}
	app.logger.Debug().Msg("Program ended.")

}

// TODO: add epoch fns
func openDB(cfg config) (*sql.DB, error) {
	db, err := sql.Open("sqlite", cfg.db.dsn)
	if err != nil {
		return nil, err
	}

	if err := data.EnableSQLiteFK(db); err != nil {
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
