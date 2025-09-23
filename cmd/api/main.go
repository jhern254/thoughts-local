package main

import (
    "fmt"
    "os"
    "net/http"
    "flag"
//    "time"

    "github.com/rs/zerolog" 
    "github.com/jhern254/go-thoughts/internal/data"
)

const dbFileName = "thoughts.db.json"
const version    = "0.1.0"

type config struct {
    port int
    env string
}

type application struct {
    config config
    logger zerolog.Logger     // fix
}

func main() {
    var cfg config

    flag.IntVar(&cfg.port, "port", 7777, "API server port")
    flag.StringVar(&cfg.env, "env", "development", "Environment (development|staging|production)")
    flag.Parse()

    // configure package zerolog
    output := zerolog.ConsoleWriter{
        Out: os.Stdout,     // for stdout 
        TimeFormat: "2006-01-02 15:04:05",
    }

    logger := zerolog.New(output).
        With().
        Timestamp().
        Caller().   // adds file and line number
        Logger().
        Level(zerolog.InfoLevel) // set log level 

    app := &application{
        config: cfg,
        logger: logger,
    }

    db, err := os.OpenFile(dbFileName, os.O_RDWR|os.O_CREATE, 0666)

    if err != nil {
        app.logger.Fatal().
            Err(err).
            Str("dbfile", dbFileName).
            Msg("problem opening dbfile")
    }

    store, err := data.NewFileSystemThoughtStore(db)
    if err != nil {
        app.logger.Fatal().Err(err).Msg("problem creating file system thought store")
    }

    server := NewThoughtServer(store)

    addr := fmt.Sprintf("%d", cfg.port)
    app.logger.Info().Str("addr", addr).Msg("starting server")

    if err := http.ListenAndServe(addr, server); err != nil {
    	app.logger.Fatal().Err(err).Str("addr", addr).Msg("ListenAndServe failed")
    }
    app.logger.Debug().Msg("Program ended.")

}
