// healthcheck.go
package main

import (
	//    "fmt"
	//    "os"
	"net/http"
	//    "errors"
	//    "strings"
	//    "io"
	//    "encoding/json"
	//    "github.com/rs/zerolog"
)

func (s *application) healthcheckHandler(w http.ResponseWriter, r *http.Request) {
	env := envelope{
		"status": "available",
		"system_info": map[string]string{
			"environment": s.config.env,
			"version":     version,
		},
	}

	err := s.writeJSON(w, http.StatusOK, env, nil)
	if err != nil {
		s.serverErrorResponse(w, r, err)
	}
}
