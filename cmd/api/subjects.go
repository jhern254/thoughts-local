// subjects.go 
// handler logic
package main

import (
    "fmt"
//    "os"
    "net/http"
    "errors"
    "strings"
    "io"
    "time"
//    "encoding/json"

//    "github.com/rs/zerolog" 
    "github.com/julienschmidt/httprouter" 
    "github.com/jhern254/go-thoughts/internal/data"
//    "github.com/jhern254/go-thoughts/internal/validator"
)

// Public methods
// TODO: implement auth for userID and db for subjectID
func (s *application) showSubjectHandler(w http.ResponseWriter, r *http.Request) {
    // NOTE: better have with db
//    id, err := s.readIDParam(r)
//    if err != nil {
//        http.NotFound(w, r)
//        return
//    }
    params := httprouter.ParamsFromContext(r.Context())
    subject := params.ByName("subject")
    userID := s.userFromReq(r)

    // query store w/ context
    subj, err := s.store.GetSubject(r.Context(), userID, subject)
    if err != nil {
        if errors.Is(err, data.ErrRecordNotFound) {
            s.notFoundResponse(w, r)    // 404
            return
        }
        s.serverErrorResponse(w, r, err) // 500
        return
    }

    // response view model
    resp := subjectResponse{
        SubjectID:  subj.SubjectID,          
        UserID:     subj.UserID,
        SubjectName: subj.SubjectName,
        CreatedAt:  subj.CreatedAt.UTC().Format(time.RFC3339),
        UpdatedAt:  subj.UpdatedAt.UTC().Format(time.RFC3339),
    }

    // handle err to json
    if err := s.writeJSON(w, http.StatusOK, envelope{"subject": resp}, nil); err != nil { 
        s.serverErrorResponse(w, r, err)
        return
    }
}

// TODO: finish and test
func (a *application) createSubjectHandler(w http.ResponseWriter, r *http.Request) {
    // endpoint-specific DTO
    var inputSubj subjectCreateRequest

    err := a.readJSON(w, r, &inputSubj)
    if err != nil {
        a.badRequestResponse(w, r, err)
        return
    }

    // init validator
//    v := validator.New
//    // TODO: write validations
//    // Check input validate subject 
//    Validator.ValidateSubject(v, &in, time.Now.UTC(), /*requireIDs=*/false)
//    if !v.Valid() {
//        a.failedValidationResponse(w, r, v.Errors)
//    }

    	// set server timestamps
//    now := time.Now().UTC()
//    in.CreatedAt = now.Format(time.RFC3339)
//    in.UpdatedAt = now.Format(time.RFC3339)
//
    // continue: persist to SQLite...

    // Map to domain
    now := time.Now().UTC()
    subj := &data.Subject{
        // SubjectID from DB
        SubjectID:   0,          // TODO: temp
        UserID:      inputSubj.UserID,
        SubjectName: inputSubj.SubjectName,
        CreatedAt:   now, 
        UpdatedAt:   now, 
    }

    // respond
    resp := subjectResponse{
        SubjectID:   subj.SubjectID,
        UserID:      subj.UserID,
        SubjectName: subj.SubjectName,
        CreatedAt:   subj.CreatedAt.Format(time.RFC3339),
        UpdatedAt:   subj.UpdatedAt.Format(time.RFC3339),
    }

    err = a.writeJSON(w, http.StatusAccepted, envelope{"subject": resp}, nil)
    // handle err to json
    if err != nil {
        a.serverErrorResponse(w, r, err)
        return
    }
}

// NOTE: for all subject thoughts 
// TODO: make id GET
// TODO: implement with thoughts model
func (s *application) showSubjectThoughtsHandler(w http.ResponseWriter, r *http.Request) {
    params := httprouter.ParamsFromContext(r.Context())
    subject := params.ByName("subject")

    userID := s.userFromReq(r)

    // TODO: temp mock thoughts model
    thoughts := s.store.GetThoughts(userID, subject)
    if thoughts == nil {
//        w.WriteHeader(http.StatusNotFound)
        s.notFoundResponse(w, r)
        return
    }

    // `map to domain
    now := time.Now().UTC()
    subj := data.Subject{
        SubjectID:      0,      // TODO: replace with db process   
        UserID:         userID,
        SubjectName:    subject,
        CreatedAt:      now,  
        UpdatedAt:      now,  
        Thoughts:       nil,
    }

    // response view model
    resp := subjectThoughtResponse{
        SubjectID:  0,   // TODO: temp, replace
        UserID:     userID,
        SubjectName: subject,
        CreatedAt:  subj.CreatedAt.Format(time.RFC3339),
        UpdatedAt:  subj.UpdatedAt.Format(time.RFC3339),
        Thoughts:   thoughts,
    }
    // TODO: test
    err := s.writeJSON(w, http.StatusOK, envelope{"subject": resp}, nil)
    // handle err to json
    if err != nil {
        s.serverErrorResponse(w, r, err)
        return
    }
}

// TODO: refactor 
func (s *application) createSubjectThoughtHandler(w http.ResponseWriter, r *http.Request) {
    params := httprouter.ParamsFromContext(r.Context())
    subject := params.ByName("subject")

    // mock
    userID := s.userFromReq(r)

    thought, err:= readThought(r.Body)
    if err != nil {
        http.Error(w, err.Error() , http.StatusBadRequest) 
        return
    }

    s.store.CaptureThought(userID, subject, thought)
    w.WriteHeader(http.StatusAccepted)
    // response view model
//    subj := subjectThoughtResponse{
//        SubjectID:  0,   // TODO: temp, replace
//        UserID:     userID,
//        SubjectName: subject,
//        CreatedAt:  time.Now(),
//        UpdatedAt:  time.Now(),
//        Thoughts:   thought,
//    }
//    fmt.Fprint(w, strings.Join(subj.Thoughts, "\n"))
}


// helper code
// --------------

// cleans input
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



