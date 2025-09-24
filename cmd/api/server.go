// server.go 
package main

import (
    "fmt"
    "os"
    "net/http"
//    "errors"
    "strings"
    "io"
//    "encoding/json"

    "github.com/rs/zerolog" 
    "github.com/julienschmidt/httprouter" 
)

const jsonContentType = "application/json"

type Subject struct {
    Name string
    Thoughts []string
    // TODO: add tags yet?
}

// Public methods
type ThoughtStore interface {
    GetThoughts(userID, subject string) []string
    CaptureThought(userID, subject, thought string)
}

// main DI container
type ThoughtServer struct {
    store       ThoughtStore
    userFromReq func(*http.Request) string
    config      config
    logger      zerolog.Logger     
}
// NOTE: pattern is server fns here, store interface done on test, main

// ctor
// NOTE: store interface already reference value, impl needs pointer
func NewThoughtServer(store ThoughtStore, cfg config, logger zerolog.Logger) *ThoughtServer {
    return &ThoughtServer{
        store:      store,
        userFromReq: func(*http.Request) string { return "test-user" }, // NOTE: default for now
        config: cfg,
        logger: logger,
    }
}

func (s *ThoughtServer) subjectsHandler(w http.ResponseWriter, r *http.Request) {
    params := httprouter.ParamsFromContext(r.Context())
    subject := params.ByName("subject")

    switch r.Method {
    case http.MethodPost:
        s.processThought(w, r, subject)
    case http.MethodGet:
        s.showThought(w, r, subject)
    default:
        http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
    }
}

func (s *ThoughtServer) showThought(w http.ResponseWriter, r *http.Request, subject string) {
    userID := s.userFromReq(r)

    thoughts := s.store.GetThoughts(userID, subject)
    if thoughts == nil {
        w.WriteHeader(http.StatusNotFound)
        return
    }
    fmt.Fprint(w, strings.Join(thoughts, "\n"))
}

func (s *ThoughtServer) processThought(w http.ResponseWriter, r *http.Request, subject string) {
    userID := s.userFromReq(r)

    thought, err:= readThought(r.Body)
    if err != nil {
        http.Error(w, err.Error() , http.StatusBadRequest) 
        return
    }

    s.store.CaptureThought(userID, subject, thought)
    w.WriteHeader(http.StatusAccepted)
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

//func parseUserStatePath(path string) (string, bool) {
//    parts := strings.Split(strings.Trim(path, "/"), "/")
//    // expect /users/{id}/state
//    if len(parts) >= 3 && parts[0] == "users" && parts[2] == "state" {
//        return parts[1], true
//    }
//    return "", false
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

