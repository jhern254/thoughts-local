package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/rs/zerolog"
)

func TestHTTP_DiagnosticOutput(t *testing.T) {
	const private = "PRIVATE-DIAGNOSTIC-MARKER"
	for _, tc := range []struct {
		name    string
		failure error
		status  int
	}{
		{"duplicate wrapper", fmt.Errorf(private+": %w", data.ErrDuplicateRecord), 409},
		{"unknown failure", errors.New(private), 500},
		{"mixed failure", errors.Join(data.ErrRecordNotFound, errors.New(private)), 500},
		{"untrusted validation", &subject.ValidationError{Fields: map[string]string{private: private, "subject_name": private}}, 422},
	} {
		t.Run(tc.name+" uses safe diagnostics", func(t *testing.T) {
			var logs bytes.Buffer
			store := &subjectStoreStub{createSubject: func(context.Context, *data.Subject) (*data.Subject, error) { return nil, tc.failure }}
			app := newSubjectServer(store)
			app.logger = zerolog.New(&logs)
			response := httptest.NewRecorder()
			app.routes().ServeHTTP(response, subjectRequest(http.MethodPost, "/subjects?private="+private, `{"subject_name":"`+private+`"}`))
			if got, want := response.Code, tc.status; got != want {
				t.Fatalf("got status %d, want %d", got, want)
			}
			for stream, output := range map[string]string{"response": response.Body.String(), "logs": logs.String()} {
				if strings.Contains(output, private) {
					t.Fatalf("got %s containing private marker, want safe diagnostic", stream)
				}
			}
			if tc.status == 500 && logs.Len() == 0 {
				t.Fatal("got no failure log, want safe operational event")
			}
		})
	}
	t.Run("unknown JSON keys do not enter responses", func(t *testing.T) {
		app := newSubjectServer(&subjectStoreStub{})
		response := httptest.NewRecorder()
		app.routes().ServeHTTP(response, subjectRequest(http.MethodPost, "/subjects", `{"`+private+`":"value"}`))
		if got, want := response.Code, 400; got != want {
			t.Fatalf("got status %d, want %d", got, want)
		}
		if strings.Contains(response.Body.String(), private) {
			t.Fatal("got private key in response, want fixed guidance")
		}
	})
	t.Run("successful subject content remains intentional output", func(t *testing.T) {
		store := &subjectStoreStub{createSubject: func(_ context.Context, item *data.Subject) (*data.Subject, error) {
			item.SubjectID = 1
			return item, nil
		}}
		app := newSubjectServer(store)
		response := httptest.NewRecorder()
		app.routes().ServeHTTP(response, subjectRequest(http.MethodPost, "/subjects", `{"subject_name":"`+private+`"}`))
		var body struct {
			Subject subjectResponse `json:"subject"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if got, want := body.Subject.SubjectName, private; got != want {
			t.Fatalf("got subject name %q, want %q", got, want)
		}
	})
}

func TestHTTP_ThoughtDiagnostics(t *testing.T) {
	const private = "PRIVATE-DIAGNOSTIC-MARKER"
	t.Run("preserves requested thought content separately from logs", func(t *testing.T) {
		var logs bytes.Buffer
		app := newThoughtServer(&thoughtStoreStub{getThought: func(context.Context, string, int64) (*data.Thought, error) {
			return &data.Thought{ThoughtID: 1, Thought: private}, nil
		}})
		app.logger = zerolog.New(&logs)
		response := httptest.NewRecorder()
		app.routes().ServeHTTP(response, thoughtRequest(http.MethodGet, "/thoughts/1", ""))
		if got, want := response.Code, http.StatusOK; got != want {
			t.Fatalf("got status %d, want %d", got, want)
		}
		if !strings.Contains(response.Body.String(), private) || strings.Contains(logs.String(), private) {
			t.Fatalf("got response=%q logs=%q, want content only in response", response.Body.String(), logs.String())
		}
	})
	for _, tc := range []struct {
		name   string
		fields map[string]string
	}{
		{"previously approved validation", map[string]string{"thought": "must be provided"}},
		{"private validation details", map[string]string{private: private, "thought": private}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := newThoughtServer(&thoughtStoreStub{createThought: func(context.Context, *data.Thought) (*data.Thought, error) {
				return nil, fmt.Errorf(private+": %w", &thought.ValidationError{Fields: tc.fields})
			}})
			response := httptest.NewRecorder()
			app.routes().ServeHTTP(response, thoughtRequest(http.MethodPost, "/thoughts", `{"thought":"`+private+`"}`))
			if got, want := response.Code, http.StatusUnprocessableEntity; got != want {
				t.Fatalf("got status %d, want %d", got, want)
			}
			var body bytes.Buffer
			if err := json.Compact(&body, response.Body.Bytes()); err != nil {
				t.Fatal(err)
			}
			if got, want := body.String(), `{"error":{"request":"The submitted values are invalid."}}`; got != want {
				t.Fatalf("got response %q, want %q", got, want)
			}
		})
	}
}

func TestHTTP_ServerDiagnostics(t *testing.T) {
	t.Run("replaces formatted library diagnostics with one safe event", func(t *testing.T) {
		var output bytes.Buffer
		logger := log.New(serverDiagnosticWriter{logger: zerolog.New(&output)}, "", 0)
		logger.Print("PRIVATE-DIAGNOSTIC-MARKER")
		if strings.Contains(output.String(), "PRIVATE-DIAGNOSTIC-MARKER") {
			t.Fatal("got private server diagnostic, want metadata only")
		}
		if got, want := strings.Count(output.String(), "HTTP server diagnostic"), 1; got != want {
			t.Fatalf("got events %d, want %d", got, want)
		}
	})
}
