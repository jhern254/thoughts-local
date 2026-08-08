package main

import (
	"errors"
	"net/http"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
)

func subjectDTO(subject *data.Subject) subjectResponse {
	return subjectResponse{
		SubjectID:   subject.SubjectID,
		UserID:      subject.UserID,
		SubjectName: subject.SubjectName,
		CreatedAt:   subject.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   subject.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func (a *application) showSubjectHandler(w http.ResponseWriter, r *http.Request) {
	id, err := a.readIDParam(r)
	if err != nil {
		a.notFoundResponse(w, r)
		return
	}
	item, err := a.subjects.GetSubject(r.Context(), a.userFromReq(r), id)
	if errors.Is(err, data.ErrRecordNotFound) {
		a.notFoundResponse(w, r)
		return
	}
	if err != nil {
		a.serverErrorResponse(w, r, err)
		return
	}
	if err := a.writeJSON(w, http.StatusOK, envelope{"subject": subjectDTO(item)}, nil); err != nil {
		a.serverErrorResponse(w, r, err)
	}
}

func (a *application) createSubjectHandler(w http.ResponseWriter, r *http.Request) {
	var input subjectCreateRequest
	if err := a.readJSON(w, r, &input); err != nil {
		a.badRequestResponse(w, r, err)
		return
	}
	item, err := a.subjectService.Create(r.Context(), a.userFromReq(r), input.SubjectName)
	var validationErr *subject.ValidationError
	switch {
	case errors.As(err, &validationErr):
		a.failedValidationResponse(w, r, validationErr.Fields)
	case errors.Is(err, data.ErrDuplicateRecord):
		a.duplicateRecordResponse(w, r, input.SubjectName)
	case err != nil:
		a.serverErrorResponse(w, r, err)
	default:
		if err := a.writeJSON(w, http.StatusCreated, envelope{"subject": subjectDTO(item)}, nil); err != nil {
			a.serverErrorResponse(w, r, err)
		}
	}
}
