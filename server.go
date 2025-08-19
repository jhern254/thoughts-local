// server.go 
package main

import (
    "fmt"
    "os"
    "net/http"
//    "errors"
    "strings"
    "io"

    "github.com/rs/zerolog" 
)

// Public methods
type ThoughtStore interface {
    GetThoughts(subject string) []string
    CaptureThought(subject, thought string)
}

type ThoughtServer struct {
    store ThoughtStore
}
// NOTE: pattern is server fns here, store interface done on test, main

// method needs pointer as input
func (s *ThoughtServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    router := http.NewServeMux()
    router.Handle("/stats", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    }))

    router.Handle("/subjects/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        switch r.Method {
        case http.MethodPost:
            s.processThought(w, r)
        case http.MethodGet:
            s.showThought(w, r)
        }
    }))

    router.ServeHTTP(w, r)
}

func (s *ThoughtServer) showThought(w http.ResponseWriter, r *http.Request) {
    subject := strings.ToLower(strings.TrimPrefix(r.URL.Path, "/subjects/"))

    thoughts := s.store.GetThoughts(subject)
    if thoughts == nil {
        w.WriteHeader(http.StatusNotFound)
    }
    fmt.Fprint(w, strings.Join(thoughts, "\n"))
}

func (s *ThoughtServer) processThought(w http.ResponseWriter, r *http.Request) {
    subject := strings.ToLower(strings.TrimPrefix(r.URL.Path, "/subjects/"))

    body, err := io.ReadAll(r.Body)
    if err != nil {
        http.Error(w, "failed to read body", http.StatusBadRequest) 
        return
    }
    defer r.Body.Close()

    thought := strings.TrimSpace(string(body))
    if thought == "" {
        http.Error(w, "empty thought", http.StatusBadRequest)
        return
    }

    s.store.CaptureThought(subject, thought)
    w.WriteHeader(http.StatusAccepted)
}

//func (s *ThoughtServer) Stats(w http.ResponseWriter, r *http.Request) {
//    w.WriteHeader(http.StatusAccepted)
//}


func Run() {
    // configure package zerolog
    output := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "2006-01-02 15:04:05"}
    logger := zerolog.New(output).With().
        Timestamp().
        Caller().   // adds file and line number
        Logger().Level(zerolog.ErrorLevel) // set log level 

    logger.Debug().Msg("Program ended.")
}

