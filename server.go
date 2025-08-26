// server.go 
package main

import (
    "fmt"
    "os"
    "net/http"
//    "errors"
    "strings"
    "io"
    "encoding/json"

    "github.com/rs/zerolog" 
)

type Subject struct {
    Name string
    Thought string
    // TODO: add tags yet?
}

type UserState struct {
    UserID  string
    Subjects []Subject
}

// Public methods
type ThoughtStore interface {
    GetThoughts(subject string) []string
    CaptureThought(subject, thought string)
}

type ThoughtServer struct {
    store ThoughtStore
    router *http.ServeMux
}
// NOTE: pattern is server fns here, store interface done on test, main

// ctor
// NOTE: store interface already reference value, impl needs pointer
func NewThoughtServer(store ThoughtStore) *ThoughtServer {
    s := &ThoughtServer{
        store,
        http.NewServeMux(),
    }

    s.router.Handle("/users/", http.HandlerFunc(s.usersHandler)) 
    s.router.Handle("/subjects/", http.HandlerFunc(s.subjectsHandler))

    return s
}

// method needs pointer as input
func (s *ThoughtServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    s.router.ServeHTTP(w, r)
}

func (s *ThoughtServer) usersHandler(w http.ResponseWriter, r *http.Request) {
    userID, ok := parseUserStatePath(r.URL.Path)
    if !ok {
        http.NotFound(w, r)
        return
    }

    w.Header().Set("content-type", "application/json")
    w.WriteHeader(http.StatusOK)
    
    userTable := UserState{
        UserID: userID,
        Subjects: []Subject{
            {"Physics", "Idk physics"},
        },
    }

    _ = json.NewEncoder(w).Encode(userTable)
}

func (s *ThoughtServer) subjectsHandler(w http.ResponseWriter, r *http.Request) {
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

func parseUserStatePath(path string) (string, bool) {
    parts := strings.Split(strings.Trim(path, "/"), "/")
    // expect /users/{id}/state
    if len(parts) >= 3 && parts[0] == "users" && parts[2] == "state" {
        return parts[1], true
    }
    return "", false
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

