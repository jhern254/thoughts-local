// subjects.go
// handler logic
package main

import (
	"fmt"
	//    "os"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	//    "encoding/json"

	//    "github.com/rs/zerolog"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/julienschmidt/httprouter"
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
			s.notFoundResponse(w, r) // 404
			return
		}
		s.serverErrorResponse(w, r, err) // 500
		return
	}

	// response view model
	resp := subjectResponse{
		SubjectID:   subj.SubjectID,
		UserID:      subj.UserID,
		SubjectName: subj.SubjectName,
		CreatedAt:   subj.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   subj.UpdatedAt.UTC().Format(time.RFC3339),
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
	if err := a.readJSON(w, r, &inputSubj); err != nil {
		a.badRequestResponse(w, r, err)
		return
	}
	userID := a.userFromReq(r)

	subj, serviceErr := a.subjectService.Create(r.Context(), userID, inputSubj.SubjectName)
	if serviceErr != nil {
		var validationErr *subject.ValidationError
		switch {
		case errors.As(serviceErr, &validationErr):
			a.failedValidationResponse(w, r, validationErr.Fields)
			return
		case errors.Is(serviceErr, data.ErrDuplicateRecord):
			a.duplicateRecordResponse(w, r, inputSubj.SubjectName) // 409
			return
		default:
			a.serverErrorResponse(w, r, serviceErr) // 500
			return
		}
	}

	// respond
	resp := subjectResponse{
		SubjectID:   subj.SubjectID,
		UserID:      subj.UserID,
		SubjectName: subj.SubjectName,
		CreatedAt:   subj.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   subj.UpdatedAt.Format(time.RFC3339),
	}

	// handle err to json
	if err := a.writeJSON(w, http.StatusCreated, envelope{"subject": resp}, nil); err != nil {
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
		SubjectID:   0, // TODO: replace with db process
		UserID:      userID,
		SubjectName: subject,
		CreatedAt:   now,
		UpdatedAt:   now,
		Thoughts:    nil,
	}

	// response view model
	resp := subjectThoughtResponse{
		SubjectID:   0, // TODO: temp, replace
		UserID:      userID,
		SubjectName: subject,
		CreatedAt:   subj.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   subj.UpdatedAt.Format(time.RFC3339),
		Thoughts:    thoughts,
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

	thought, err := readThought(r.Body)
	if err != nil {
		s.serverErrorResponse(w, r, err) // 500
		return
	}

	thID, tErr := s.store.CaptureThought(r.Context(), userID, subject, thought)
	if tErr != nil {
		s.serverErrorResponse(w, r, tErr) // 500
		return
	}

	// temp domain mapping
	now := time.Now().UTC()
	th := struct {
		ThoughtID int64
		UserID    string
		SubjectID int64
		EventID   int64
		Thought   string
		CreatedAt time.Time
		UpdatedAt time.Time
	}{
		ThoughtID: thID,
		UserID:    userID,
		SubjectID: 0, // NOTE: temp
		EventID:   0,
		Thought:   thought,
		CreatedAt: now,
		UpdatedAt: now,
	}
	//    w.WriteHeader(http.StatusAccepted)

	// TODO: add validator

	// temp response mapping
	resp := struct {
		ThoughtID int64  `json:"thought_id"`
		UserID    string `json:"user_id"`
		SubjectID int64  `json:"subject_id"`
		EventID   int64  `json:"event_id"`
		Thought   string `json:"thought"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"-"`
	}{
		ThoughtID: th.ThoughtID,
		UserID:    th.UserID,
		SubjectID: th.SubjectID,
		EventID:   th.EventID,
		Thought:   th.Thought,
		CreatedAt: th.CreatedAt.Format(time.RFC3339),
		UpdatedAt: th.UpdatedAt.Format(time.RFC3339),
	}

	// handle err to json
	if err = s.writeJSON(w, http.StatusCreated, envelope{"thought": resp}, nil); err != nil {
		s.serverErrorResponse(w, r, err)
		return
	}
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
