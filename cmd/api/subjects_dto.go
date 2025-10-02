// subjects_dto.go 
// request transport layer before domain object
// response view model
package main

import (
//    "fmt"
//    "os"
//    "net/http"
//    "errors"
//    "strings"
//    "io"
//    "time"
//    "encoding/json"

//    "github.com/rs/zerolog" 
//    "github.com/julienschmidt/httprouter" 
//    "github.com/jhern254/go-thoughts/internal/data"
//    "github.com/jhern254/go-thoughts/internal/validator"
)

// Create:
// NOTE: client should NOT send IDs or server timestamps.
type subjectCreateRequest struct {
    UserID      string  `json:"user_id"`        // required
    SubjectName string  `json:"subject_name"`   // required
    // Optional: allow bulk-thoughts on create (limit them in validation)
    Thoughts    []string `json:"thoughts,omitempty"`
}

// update, pointers for omitted
type subjectUpdateRequest struct {
    SubjectName *string  `json:"subject_name,omitempty"`
    // Thoughts updates usually via separate endpoint; include only if you support it here.
    // Thoughts     *[]string `json:"thoughts,omitempty"`
}

// response dto, view model
type subjectResponse struct {
    SubjectID   int64       `json:"subject_id"`
    UserID      string      `json:"user_id"`
    SubjectName string      `json:"subject_name"`
    CreatedAt   string      `json:"created_at"`  // RFC3339
    UpdatedAt   string      `json:"updated_at"`  // RFC3339
}

type subjectThoughtResponse struct {
    SubjectID   int64       `json:"subject_id"`
    UserID      string      `json:"user_id"`
    SubjectName string      `json:"subject_name"`
    CreatedAt   string      `json:"created_at"`  // RFC3339
    UpdatedAt   string      `json:"updated_at"`  // RFC3339
    Thoughts    []string    `json:"thoughts,omitempty"`
}


