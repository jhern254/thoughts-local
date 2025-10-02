// subjects.go
// domain model
package data

import (
//    "fmt"
//    "os"
    "time"
//    "errors"
//    "strings"
//    "io"
    "context"

//    "github.com/rs/zerolog" 
//    "github.com/julienschmidt/httprouter" 
)

type Subject struct {
    SubjectID   int64     `json:"subject_id"`
    UserID      string    `json:"user_id"`
    SubjectName string    `json:"subject_name"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"-"`

    // TODO: TEMP, refactor out to model
    Thoughts []string   `json:"thoughts,omitempty"`
}

// Stub test interfaces
type ThoughtStore interface {
    // NOTE: will be db wrapper fn
    GetThoughts(userID, subject string) []string
    CaptureThought(userID, subject, thought string)
}

type SubjectStore interface {
    GetSubject(ctx context.Context, userID, subject string) (*Subject, error)
    CaptureSubject(ctx context.Context, userID, subject string) (int64, error)
}

