package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/rs/zerolog"
)

func thoughtRequest(method, path, body string) *http.Request {
	return httptest.NewRequest(method, path, bytes.NewBufferString(body))
}

type thoughtStoreStub struct {
	createThought func(context.Context, *data.Thought) (*data.Thought, error)
	getThought    func(context.Context, string, int64) (*data.Thought, error)
	createCalls   int
}

func (s *thoughtStoreStub) CreateThought(ctx context.Context, item *data.Thought) (*data.Thought, error) {
	s.createCalls++
	if s.createThought == nil {
		panic("CreateThought not stubbed")
	}
	return s.createThought(ctx, item)
}

func (s *thoughtStoreStub) GetThought(ctx context.Context, userID string, thoughtID int64) (*data.Thought, error) {
	if s.getThought == nil {
		panic("GetThought not stubbed")
	}
	return s.getThought(ctx, userID, thoughtID)
}

func newThoughtServer(store thought.Store) *application {
	return NewApplication(subject.NewService(testutils.NewFakeSubjectStore()), thought.NewService(store), config{}, zerolog.New(io.Discard))
}

func TestShowThoughtHandler(t *testing.T) {
	t.Run("returns 200 with thought", func(t *testing.T) {
		now := time.Date(2026, time.August, 1, 12, 0, 0, 0, time.UTC)
		subjectID := int64(3)
		store := testutils.NewFakeThoughtStore()
		created, err := store.CreateThought(context.Background(), &data.Thought{UserID: "test-user", SubjectID: &subjectID, Thought: "learn Go", Version: 1, ObservedAt: now, CreatedAt: now, UpdatedAt: now})
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()

		newThoughtServer(store).routes().ServeHTTP(response, thoughtRequest(http.MethodGet, "/thoughts/1", ""))

		testutils.AssertStatusCode(t, response.Code, http.StatusOK)
		testutils.AssertContentType(t, response, jsonContentType)
		var body struct {
			Thought thoughtResponse `json:"thought"`
		}
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		testutils.AssertCorrect(t, body.Thought.ThoughtID, created.ThoughtID)
		testutils.AssertCorrect(t, body.Thought.Thought, "learn Go")
		testutils.AssertCorrect(t, *body.Thought.SubjectID, subjectID)
	})

	t.Run("returns 404 for missing thought", func(t *testing.T) {
		store := testutils.NewFakeThoughtStore()
		response := httptest.NewRecorder()

		newThoughtServer(store).routes().ServeHTTP(response, thoughtRequest(http.MethodGet, "/thoughts/99", ""))

		testutils.AssertStatusCode(t, response.Code, http.StatusNotFound)
	})

	t.Run("returns 404 for invalid thought ID", func(t *testing.T) {
		response := httptest.NewRecorder()

		newThoughtServer(testutils.NewFakeThoughtStore()).routes().ServeHTTP(response, thoughtRequest(http.MethodGet, "/thoughts/not-an-id", ""))

		testutils.AssertStatusCode(t, response.Code, http.StatusNotFound)
	})

	t.Run("returns 500 when store fails", func(t *testing.T) {
		store := &thoughtStoreStub{getThought: func(context.Context, string, int64) (*data.Thought, error) { return nil, errors.New("boom") }}
		response := httptest.NewRecorder()

		newThoughtServer(store).routes().ServeHTTP(response, thoughtRequest(http.MethodGet, "/thoughts/7", ""))

		testutils.AssertStatusCode(t, response.Code, http.StatusInternalServerError)
	})
}

func TestCreateThoughtHandler(t *testing.T) {
	t.Run("returns 201 with created thought", func(t *testing.T) {
		store := testutils.NewFakeThoughtStore()
		response := httptest.NewRecorder()

		newThoughtServer(store).routes().ServeHTTP(response, thoughtRequest(http.MethodPost, "/thoughts", `{"subject_id":3,"thought":"learn Go","observed_at":"2026-08-01T12:00:00Z"}`))

		testutils.AssertStatusCode(t, response.Code, http.StatusCreated)
		testutils.AssertContentType(t, response, jsonContentType)
		var body struct {
			Thought thoughtResponse `json:"thought"`
		}
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		testutils.AssertCorrect(t, body.Thought.ThoughtID, int64(1))
		testutils.AssertCorrect(t, body.Thought.Thought, "learn Go")
		testutils.AssertCorrect(t, body.Thought.Version, int64(1))
		testutils.AssertCorrect(t, body.Thought.ObservedAt, "2026-08-01T12:00:00Z")
	})

	t.Run("returns 400 for malformed JSON", func(t *testing.T) {
		response := httptest.NewRecorder()

		newThoughtServer(testutils.NewFakeThoughtStore()).routes().ServeHTTP(response, thoughtRequest(http.MethodPost, "/thoughts", `{"thought":`))

		testutils.AssertStatusCode(t, response.Code, http.StatusBadRequest)
	})

	t.Run("returns 400 for invalid observed at", func(t *testing.T) {
		response := httptest.NewRecorder()

		newThoughtServer(testutils.NewFakeThoughtStore()).routes().ServeHTTP(response, thoughtRequest(http.MethodPost, "/thoughts", `{"thought":"learn Go","observed_at":"yesterday"}`))

		testutils.AssertStatusCode(t, response.Code, http.StatusBadRequest)
	})

	t.Run("returns 422 without persisting invalid thought", func(t *testing.T) {
		store := &thoughtStoreStub{}
		response := httptest.NewRecorder()

		newThoughtServer(store).routes().ServeHTTP(response, thoughtRequest(http.MethodPost, "/thoughts", `{"thought":" "}`))

		testutils.AssertStatusCode(t, response.Code, http.StatusUnprocessableEntity)
		testutils.AssertCorrect(t, store.createCalls, 0)
		if !strings.Contains(response.Body.String(), `"thought"`) {
			t.Fatalf("got response body %s", response.Body.String())
		}
	})

	t.Run("returns 404 for missing subject", func(t *testing.T) {
		store := &thoughtStoreStub{createThought: func(context.Context, *data.Thought) (*data.Thought, error) { return nil, data.ErrRecordNotFound }}
		response := httptest.NewRecorder()

		newThoughtServer(store).routes().ServeHTTP(response, thoughtRequest(http.MethodPost, "/thoughts", `{"subject_id":99,"thought":"learn Go"}`))

		testutils.AssertStatusCode(t, response.Code, http.StatusNotFound)
	})

	t.Run("returns 500 when store fails", func(t *testing.T) {
		store := &thoughtStoreStub{createThought: func(context.Context, *data.Thought) (*data.Thought, error) { return nil, errors.New("boom") }}
		response := httptest.NewRecorder()

		newThoughtServer(store).routes().ServeHTTP(response, thoughtRequest(http.MethodPost, "/thoughts", `{"thought":"learn Go"}`))

		testutils.AssertStatusCode(t, response.Code, http.StatusInternalServerError)
	})
}
