package main

import (
//    "fmt"
    "os"
    "net/http"
//    server "github.com/jhern254/go-thoughts"

    "github.com/rs/zerolog" 
)

const dbFileName = "thoughts.db.json"

func main() {
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

    db, err := os.OpenFile(dbFileName, os.O_RDWR|os.O_CREATE, 0666)

    if err != nil {
        logger.Fatal().
            Err(err).
            Str("dbfile", dbFileName).
            Msg("problem opening dbfile")
    }

    store, err := NewFileSystemThoughtStore(db)
    if err != nil {
		logger.Fatal().Err(err).Msg("problem creating file system thought store")
    }

    server := NewThoughtServer(store)

    addr := ":7777"
	logger.Info().Str("addr", addr).Msg("starting server")

	if err := http.ListenAndServe(addr, server); err != nil {
		logger.Fatal().Err(err).Str("addr", addr).Msg("ListenAndServe failed")
	}
    logger.Debug().Msg("Program ended.")

}
