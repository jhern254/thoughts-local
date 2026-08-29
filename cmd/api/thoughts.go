package main

import (
	"errors"
	"net/http"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/thought"
)

func (a *application) showThoughtHandler(w http.ResponseWriter, r *http.Request) {
	id, err := a.readIDParam(r)
	if err != nil {
		a.notFoundResponse(w, r)
		return
	}
	item, err := a.thoughtService.Get(r.Context(), a.userFromReq(r), id)
	if errors.Is(err, data.ErrRecordNotFound) {
		a.notFoundResponse(w, r)
		return
	}
	if err != nil {
		a.serverErrorResponse(w, r, err)
		return
	}
	if err := a.writeJSON(w, http.StatusOK, envelope{"thought": toThoughtResponse(item)}, nil); err != nil {
		a.serverErrorResponse(w, r, err)
	}
}

func (a *application) createThoughtHandler(w http.ResponseWriter, r *http.Request) {
	var input thoughtCreateRequest
	if err := a.readJSON(w, r, &input); err != nil {
		a.badRequestResponse(w, r, err)
		return
	}
	var observedAt time.Time
	if input.ObservedAt != nil {
		var err error
		observedAt, err = time.Parse(time.RFC3339, *input.ObservedAt)
		if err != nil {
			a.badRequestResponse(w, r, errors.New("observed_at must be RFC3339"))
			return
		}
	}
	item, err := a.thoughtService.Create(r.Context(), a.userFromReq(r), input.Thought, input.SubjectID, observedAt)
	var validationErr *thought.ValidationError
	switch {
	case errors.As(err, &validationErr):
		a.failedValidationResponse(w, r, validationErr.Fields)
	case errors.Is(err, data.ErrRecordNotFound):
		a.notFoundResponse(w, r)
	case err != nil:
		a.serverErrorResponse(w, r, err)
	default:
		if err := a.writeJSON(w, http.StatusCreated, envelope{"thought": toThoughtResponse(item)}, nil); err != nil {
			a.serverErrorResponse(w, r, err)
		}
	}
}
