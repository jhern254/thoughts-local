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

//    "github.com/rs/zerolog" 
)

// helpers
func (a *application) logError(r *http.Request, err error) {
    a.logger.Error().
        Err(err).
        Str("method", r.Method).
        Str("url", r.URL.String()).
        Msg("request error")
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
func (a *application) badRequestResponse(w http.ResponseWriter, r *http.Request, err error) {
    a.errorResponse(w, r, http.StatusBadRequest, err.Error())
}

func (a *application) failedValidationResponse(w http.ResponseWriter, r *http.Request, errors map[string]string) {
    a.errorResponse(w, r, http.StatusUnprocessableEntity, errors)
}

