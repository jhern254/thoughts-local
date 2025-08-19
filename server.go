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

    router.Handle("/stats", http.HandlerFunc(s.statsHandler))
    router.Handle("/subjects/", http.HandlerFunc(s.subjectsHandler))

    router.ServeHTTP(w, r)
}

func (s *ThoughtServer)statsHandler(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
}

func (s *ThoughtServer)subjectsHandler(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodPost:
        s.processThought(w, r)
    case http.MethodGet:
        s.showThought(w, r)
    default:
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
    }
}

func (s *ThoughtServer) showThought(w http.ResponseWriter, r *http.Request) {
    subject := subjectFromPath(r.URL.Path)

    thoughts := s.store.GetThoughts(subject)
    if thoughts == nil {
        w.WriteHeader(http.StatusNotFound)
    }
    fmt.Fprint(w, strings.Join(thoughts, "\n"))
}

func (s *ThoughtServer) processThought(w http.ResponseWriter, r *http.Request) {
    subject := subjectFromPath(r.URL.Path)

    thought, err:= readThought(r.Body)
    if err != nil {
        http.Error(w, err.Error() , http.StatusBadRequest) 
        return
    }

    s.store.CaptureThought(subject, thought)
    w.WriteHeader(http.StatusAccepted)
}

// helper fns
func subjectFromPath(path string) string {
    return strings.ToLower(strings.TrimPrefix(path, "/subjects/"))
}

func readThought(body io.ReadCloser) (string, error) {
    defer body.Close()
    b, err := io.ReadAll(body)
    if err != nil {
        return "", fmt.Errorf("failed to read body")
    }
    thought := strings.TrimSpace(string(b))
    if thought == "" {
        return "", fmt.Errorf("empty thought")
    }

    return thought, nil
} 

func Run() {
    // configure package zerolog
    output := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "2006-01-02 15:04:05"}
    logger := zerolog.New(output).With().
        Timestamp().
        Caller().   // adds file and line number
        Logger().Level(zerolog.ErrorLevel) // set log level 

    logger.Debug().Msg("Program ended.")
}

