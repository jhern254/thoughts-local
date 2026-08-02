// subjects.go
// domain model
package data

import (
	"fmt"
	//    "os"
	"time"
	//    "errors"
	"strings"
	//    "io"
	"context"

	//    "github.com/rs/zerolog"
	//    "github.com/julienschmidt/httprouter"
	"github.com/jhern254/go-thoughts/internal/validator"
)

const (
	maxSubjectNameBytes = 120
	maxThoughtsOnCreate = 50
)

type Subject struct {
	SubjectID   int64     `json:"subject_id"`
	UserID      string    `json:"user_id"`
	SubjectName string    `json:"subject_name"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"-"`

	// TODO: TEMP, refactor out to model
	Thoughts []string `json:"thoughts,omitempty"`
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

// Validation fns
// normalize trims and collapses internal whitespace to single spaces.
func normalize(s string) string {
	s = strings.TrimSpace(s)
	return strings.Join(strings.Fields(s), " ")
}

// Validate for POST /subjects
func ValidateSubjectCreate(v *validator.Validator, in *Subject) {
	// subject_id
	if in.SubjectID == 0 {
		v.AddError("subject_id", "must be provided")
	}

	// user_id
	v.Check(in.UserID != "", "user_id", "must be provided")

	// subject_name: required, trimmed, length cap
	name := normalize(in.SubjectName)
	v.Check(name != "", "subject_name", "must be provided")
	v.Check(len(name) <= maxSubjectNameBytes, "subject_name", "must not be more than 120 bytes long")
	// clean
	in.SubjectName = name

	// optional: thoughts batch on create (safe caps)
	if in.Thoughts != nil {
		v.Check(len(in.Thoughts) <= maxThoughtsOnCreate, "thoughts", "too many items")
		for i, t := range in.Thoughts {
			v.Check(t != "", fmt.Sprintf("thoughts[%d]", i), "must be provided")
		}
	}
}

// Validate for PATCH /subjects/:id
//func ValidateSubjectUpdate(v *validator.Validator, in *Subject) {
//    if in.SubjectName != "" {
//        name := normalize(*in.SubjectName)
//        v.Check(len(name) <= maxSubjectNameBytes, "subject_name", "must not be more than 120 bytes long")
//        *in.SubjectName = name
//    }
//}
