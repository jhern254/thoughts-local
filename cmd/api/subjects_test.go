package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/rs/zerolog"
)

func newServer() *application {
	store := data.NewInMemoryStore()
	return NewApplication(store, store, config{}, zerolog.New(io.Discard))
}
func request(method, path, body string) *http.Request {
	return httptest.NewRequest(method, path, bytes.NewBufferString(body))
}

func TestSubjectAndThoughtResourcesAreSeparate(t *testing.T) {
	server := newServer()
	createdSubject := httptest.NewRecorder()
	server.routes().ServeHTTP(createdSubject, request(http.MethodPost, "/subjects", `{"subject_name":"coding"}`))
	if createdSubject.Code != http.StatusCreated {
		t.Fatalf("subject status %d", createdSubject.Code)
	}
	var subject struct {
		Subject subjectResponse `json:"subject"`
	}
	if err := json.NewDecoder(createdSubject.Body).Decode(&subject); err != nil {
		t.Fatal(err)
	}
	createdThought := httptest.NewRecorder()
	server.routes().ServeHTTP(createdThought, request(http.MethodPost, "/thoughts", `{"subject_id":1,"thought":"learn Go"}`))
	if createdThought.Code != http.StatusCreated {
		t.Fatalf("thought status %d", createdThought.Code)
	}
	var thought struct {
		Thought thoughtResponse `json:"thought"`
	}
	if err := json.NewDecoder(createdThought.Body).Decode(&thought); err != nil {
		t.Fatal(err)
	}
	if thought.Thought.SubjectID == nil || *thought.Thought.SubjectID != subject.Subject.SubjectID || thought.Thought.Version != 1 {
		t.Fatalf("bad thought %#v", thought.Thought)
	}
	gotSubject := httptest.NewRecorder()
	server.routes().ServeHTTP(gotSubject, request(http.MethodGet, "/subjects/1", ""))
	if gotSubject.Code != http.StatusOK {
		t.Fatalf("get subject status %d", gotSubject.Code)
	}
	var body map[string]json.RawMessage
	if err := json.NewDecoder(gotSubject.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	json.Unmarshal(body["subject"], &raw)
	if _, ok := raw["thoughts"]; ok {
		t.Fatal("subject response embeds thoughts")
	}
	missing := httptest.NewRecorder()
	server.routes().ServeHTTP(missing, request(http.MethodPost, "/thoughts", `{"subject_id":99,"thought":"missing"}`))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing subject status %d", missing.Code)
	}
}

func TestNestedThoughtRoutesAreAbsent(t *testing.T) {
	server := newServer()
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, request(http.MethodGet, "/subjects/coding/thoughts", ""))
	if response.Code != http.StatusNotFound {
		t.Fatalf("got %d", response.Code)
	}
}
