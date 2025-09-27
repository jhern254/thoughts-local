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
    "time"
//    "encoding/json"

//    "github.com/rs/zerolog" 
    "github.com/julienschmidt/httprouter" 
    "github.com/jhern254/go-thoughts/internal/data"
)

// Public methods
type ThoughtStore interface {
    // NOTE: will be db wrapper fn
    GetThoughts(userID, subject string) []string
    CaptureThought(userID, subject, thought string)
}

// NOTE: for all subject thoughts 
// TODO: make id GET
// TODO: implement with thoughts model
func (s *application) showSubjectThoughtsHandler(w http.ResponseWriter, r *http.Request) {
    params := httprouter.ParamsFromContext(r.Context())
    subject := params.ByName("subject")

    userID := s.userFromReq(r)

    thoughts := s.store.GetThoughts(userID, subject)
    if thoughts == nil {
        w.WriteHeader(http.StatusNotFound)
        return
    }

    subj := data.Subject{
        SubjectID:  0,   // TODO: temp, replace
        UserID:     userID,
        Name:       subject,
        CreatedAt:  time.Now(),
        UpdatedAt:  time.Now(),
        Thoughts:   thoughts,
    }
    fmt.Fprint(w, strings.Join(subj.Thoughts, "\n"))
}

func (s *application) createSubjectThoughtHandler(w http.ResponseWriter, r *http.Request) {
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

