// errors.go
// http level error helpers
// translate go errors into json http responses (4xx, 5xx errs)
package main

import (
	//    "fmt"
	"net/http"
	//    "strings"
	//    "os"
	//    "encoding/json"
	//    "io"

	"github.com/jhern254/go-thoughts/internal/diagnostics"
	//    "github.com/rs/zerolog"
)

// helpers
func (a *application) logError(_ *http.Request, _ error) {
	a.logger.Error().Str("operation", "http_request").Msg("request error")
}

func (a *application) errorResponse(w http.ResponseWriter, r *http.Request, status int, msg any) {
	env := envelope{"error": msg}

	err := a.writeJSON(w, status, env, nil)
	if err != nil {
		a.logError(r, err)
		w.WriteHeader(500)
	}
}

// runtime error
// 500 Internal Server Error
func (a *application) serverErrorResponse(w http.ResponseWriter, r *http.Request, err error) {
	a.logError(r, err)

	msg := "the server encountered a problem and could not process your request"
	a.errorResponse(w, r, http.StatusInternalServerError, msg)
}

// 404 Not Found Error
func (a *application) notFoundResponse(w http.ResponseWriter, r *http.Request) {
	msg := "the requested resource could not be found"
	a.errorResponse(w, r, http.StatusNotFound, msg)
}

// 405 Method Not Allowed Error
func (a *application) methodNotAllowedResponse(w http.ResponseWriter, r *http.Request) {
	msg := "the requested resource could not be found"
	a.errorResponse(w, r, http.StatusMethodNotAllowed, msg)
}

// 400 Bad Request Error
// TODO: log errs
func (a *application) badRequestResponse(w http.ResponseWriter, r *http.Request, err error) {
	a.errorResponse(w, r, http.StatusBadRequest, "The request body is invalid.")
}

// 422 Unprocessable Entity
func (a *application) failedValidationResponse(w http.ResponseWriter, r *http.Request) {
	a.errorResponse(w, r, http.StatusUnprocessableEntity, map[string]string{"request": "The submitted values are invalid."})
}

// 409 Conflict (generic)
func (a *application) conflictResponse(w http.ResponseWriter, r *http.Request, msg any) {
	a.errorResponse(w, r, http.StatusConflict, msg)
}

// 409 Conflict (duplicate record convenience)
func (a *application) duplicateRecordResponse(w http.ResponseWriter, r *http.Request) {
	a.conflictResponse(w, r, diagnostics.DuplicateSubjectMessage)
}
