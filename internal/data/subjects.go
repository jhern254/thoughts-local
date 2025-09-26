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
    SubjectID int64     `json:"subject_id"`
    UserID    string    `json:"user_id"`
    Name      string    `json:"name"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
