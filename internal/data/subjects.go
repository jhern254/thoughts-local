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
