// helper_test.go
package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type thoughtPayload struct {
	Subject string `json:"subject"`
	Thought string `json:"thought"`
}

func TestReadJSON_ThoughtsPayload(t *testing.T) {
	app := &application{} // construct as needed for your project

	big := strings.Repeat("x", 1_200_000) // > 1MB; triggers MaxBytesReader
	tests := []struct {
		name    string
		body    string
		wantErr string // substring match
	}{
		{
			name:    "syntax error trailing comma",
			body:    `{"subject":"coding","thought":"hello",}`,
			wantErr: "body contains badly-formed JSON",
		},
		{
			name:    "unexpected EOF",
			body:    `{"subject":"coding","thought":"hello"`,
			wantErr: "body contains badly-formed JSON",
		},
		{
			name:    "wrong type for field",
			body:    `{"subject":123,"thought":"hello"}`,
			wantErr: `body contains incorrect JSON type for field "subject"`,
		},
		{
			name:    "empty body",
			body:    ``,
			wantErr: "body must not be empty",
		},
		{
			name:    "unknown field",
			body:    `{"rating":"PG","subject":"coding"}`,
			wantErr: `body contains unknown key "rating"`,
		},
		{
			name: "too large body",
			// use valid JSON so the size check is the reason for failure
			body:    `{"subject":"coding","thought":"` + big + `"}`,
			wantErr: "body must not be larger than",
		},
		{
			name:    "multiple JSON values",
			body:    `{"subject":"coding","thought":"one"}{"subject":"ai","thought":"two"}`,
			wantErr: "body must only contain a single JSON value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/subjects/coding/thoughts", bytes.NewBufferString(tt.body))
			w := httptest.NewRecorder()

			var dst thoughtPayload
			err := app.readJSON(w, req, &dst)

			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("got error %q; want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestReadJSON_OK(t *testing.T) {
	app := &application{}
	body := `{"subject":"coding","thought":"I’m learning Go!"}`

	req := httptest.NewRequest(http.MethodPost, "/subjects/coding/thoughts", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	var dst thoughtPayload
	if err := app.readJSON(w, req, &dst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dst.Subject != "coding" || dst.Thought != "I’m learning Go!" {
		t.Errorf("decoded payload mismatch: %+v", dst)
	}
}
