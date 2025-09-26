// subjects.go 
// handler logic
package main

import (
    "fmt"
//    "os"
    "net/http"
//    "errors"
    "strings"
    "io"
//    "encoding/json"

//    "github.com/rs/zerolog" 
    "github.com/julienschmidt/httprouter" 
//    "github.com/jhern254/go-thoughts/internal/data"
)

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

func (s *application) showSubjectHandler(w http.ResponseWriter, r *http.Request) {
    params := httprouter.ParamsFromContext(r.Context())
    subject := params.ByName("subject")

    userID := s.userFromReq(r)

    thoughts := s.store.GetThoughts(userID, subject)
    if thoughts == nil {
        w.WriteHeader(http.StatusNotFound)
        return
    }
    fmt.Fprint(w, strings.Join(thoughts, "\n"))
}

func (s *application) createSubjectsHandler(w http.ResponseWriter, r *http.Request) {
    params := httprouter.ParamsFromContext(r.Context())
    subject := params.ByName("subject")

    userID := s.userFromReq(r)

    thought, err:= readThought(r.Body)
    if err != nil {
        http.Error(w, err.Error() , http.StatusBadRequest) 
        return
    }

    s.store.CaptureThought(userID, subject, thought)
    w.WriteHeader(http.StatusAccepted)
}

// helper fns
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

