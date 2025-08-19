package main

import (
//    "fmt"
    "os"
    "net/http"
//    server "github.com/jhern254/go-thoughts"

    "github.com/rs/zerolog" 
)

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

    
    server := NewThoughtServer(NewInMemoryThoughtStore())

	logger.Info().Str("addr", ":5000").Msg("starting server")
	if err := http.ListenAndServe(":5000", server); err != nil {
		// same intent as log.Fatal: log and exit non-zero
		logger.Fatal().Err(err).Msg("ListenAndServe failed")
	}
    logger.Debug().Msg("Program ended.")

}
