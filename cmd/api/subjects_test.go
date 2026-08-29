package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/rs/zerolog"
)

func newServer() *application {
	subjectService := subject.NewService(testutils.NewFakeSubjectStore())
	thoughtService := thought.NewService(testutils.NewFakeThoughtStore())
	return NewApplication(
		subjectService,
		thoughtService,
		config{},
		zerolog.New(io.Discard),
	)
}
func request(method, path, body string) *http.Request {
	return httptest.NewRequest(method, path, bytes.NewBufferString(body))
}

func TestApplication_Routes(t *testing.T) {
	t.Run("keeps subject and thought resources separate", func(t *testing.T) {
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
	})

	t.Run("does not register nested thought routes", func(t *testing.T) {
		server := newServer()
		response := httptest.NewRecorder()
		server.routes().ServeHTTP(response, request(http.MethodGet, "/subjects/coding/thoughts", ""))
		if response.Code != http.StatusNotFound {
			t.Fatalf("got %d", response.Code)
		}
	})
}

type missingSubjectThoughtStore struct{}

func (missingSubjectThoughtStore) CreateThought(context.Context, *data.Thought) (*data.Thought, error) {
	return nil, data.ErrRecordNotFound
}

func (missingSubjectThoughtStore) GetThought(context.Context, string, int64) (*data.Thought, error) {
	return nil, data.ErrRecordNotFound
}

func TestApplication_CreateThought(t *testing.T) {
	t.Run("returns not found for missing subject", func(t *testing.T) {
		subjectService := subject.NewService(testutils.NewFakeSubjectStore())
		thoughtService := thought.NewService(missingSubjectThoughtStore{})
		server := NewApplication(
			subjectService,
			thoughtService,
			config{},
			zerolog.New(io.Discard),
		)
		response := httptest.NewRecorder()
		server.routes().ServeHTTP(response, request(http.MethodPost, "/thoughts", `{"subject_id":99,"thought":"missing"}`))
		if response.Code != http.StatusNotFound {
			t.Fatalf("got status %d, want %d", response.Code, http.StatusNotFound)
		}
	})
}
