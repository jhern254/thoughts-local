// cmd/api/subjects_validate.go
package main

import (
    "fmt"
    "strings"

    "github.com/jhern254/go-thoughts/internal/validator"
)

const (
    maxSubjectNameBytes = 120
    maxThoughtsOnCreate = 50
    maxThoughtBytes     = 1000
)

// normalize trims and collapses internal whitespace to single spaces.
func normalize(s string) string {
    s = strings.TrimSpace(s)
    return strings.Join(strings.Fields(s), " ")
}

// Validate for POST /subjects
func ValidateSubjectCreateDTO(v *validator.Validator, in *subjectCreateRequest) {
    // user_id: keep it simple for MVP — present & not absurdly long
    v.Check(in.UserID != "", "user_id", "must be provided")
    v.Check(len(in.UserID) <= 64, "user_id", "must not be more than 64 bytes long")

    // subject_name: required, trimmed, length cap
    name := normalize(in.SubjectName)
    v.Check(name != "", "subject_name", "must be provided")
    v.Check(len(name) <= maxSubjectNameBytes, "subject_name", "must not be more than 120 bytes long")
    in.SubjectName = name

    // optional: thoughts batch on create (safe caps)
    if in.Thoughts != nil {
        v.Check(len(in.Thoughts) <= maxThoughtsOnCreate, "thoughts", "too many items")
        for i := range in.Thoughts {
            t := normalize(in.Thoughts[i])
            v.Check(t != "", fmt.Sprintf("thoughts[%d]", i), "must be provided")
            v.Check(len(t) <= maxThoughtBytes, fmt.Sprintf("thoughts[%d]", i), "must not be more than 1000 bytes long")
            in.Thoughts[i] = t
        }
    }
}

// Validate for PATCH /subjects/:id
func ValidateSubjectUpdateDTO(v *validator.Validator, in *subjectUpdateRequest) {
    if in.SubjectName != nil {
        name := normalize(*in.SubjectName)
        v.Check(name != "", "subject_name", "must be provided")
        v.Check(len(name) <= maxSubjectNameBytes, "subject_name", "must not be more than 120 bytes long")
        *in.SubjectName = name
    }
}

