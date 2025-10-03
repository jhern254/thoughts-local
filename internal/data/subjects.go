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
    // TODO: Rename to create
    CaptureThought(ctx context.Context, userID, subject, thought string) (int64, error)
}

type SubjectStore interface {
    GetSubject(ctx context.Context, userID, subject string) (*Subject, error)
    CaptureSubject(ctx context.Context, userID string, subject *Subject) (int64, error)
}

