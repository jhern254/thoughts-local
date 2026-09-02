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

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subject"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/rs/zerolog"
)

type subjectStoreStub struct {
	createSubject func(context.Context, *data.Subject) (*data.Subject, error)
	getSubject    func(context.Context, string, int64) (*data.Subject, error)
	createCalls   int
}

func (s *subjectStoreStub) CreateSubject(ctx context.Context, item *data.Subject) (*data.Subject, error) {
	s.createCalls++
	if s.createSubject == nil {
		panic("CreateSubject not stubbed")
	}
	return s.createSubject(ctx, item)
}

func (s *subjectStoreStub) GetSubject(ctx context.Context, userID string, subjectID int64) (*data.Subject, error) {
	if s.getSubject == nil {
		panic("GetSubject not stubbed")
	}
	return s.getSubject(ctx, userID, subjectID)
}

func newSubjectServer(store subject.Store) *application {
	return NewApplication(subject.NewService(store), thought.NewService(testutils.NewFakeThoughtStore()), config{}, zerolog.New(io.Discard))
}

func subjectRequest(method, path, body string) *http.Request {
	return httptest.NewRequest(method, path, bytes.NewBufferString(body))
}

func TestShowSubjectHandler(t *testing.T) {
	t.Run("returns 200 with subject", func(t *testing.T) {
		store := testutils.NewFakeSubjectStore()
		created, err := store.CreateSubject(context.Background(), &data.Subject{UserID: "test-user", SubjectName: "coding"})
		if err != nil {
			t.Fatal(err)
		}
		response := httptest.NewRecorder()

		newSubjectServer(store).routes().ServeHTTP(response, subjectRequest(http.MethodGet, "/subjects/1", ""))

		testutils.AssertStatusCode(t, response.Code, http.StatusOK)
		testutils.AssertContentType(t, response, jsonContentType)
		var body struct {
			Subject subjectResponse `json:"subject"`
		}
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		testutils.AssertCorrect(t, body.Subject.SubjectID, created.SubjectID)
		testutils.AssertCorrect(t, body.Subject.SubjectName, "coding")
	})

	t.Run("returns 404 for missing subject", func(t *testing.T) {
		store := testutils.NewFakeSubjectStore()
		response := httptest.NewRecorder()

		newSubjectServer(store).routes().ServeHTTP(response, subjectRequest(http.MethodGet, "/subjects/99", ""))

		testutils.AssertStatusCode(t, response.Code, http.StatusNotFound)
	})

	t.Run("returns 404 for invalid subject ID", func(t *testing.T) {
		response := httptest.NewRecorder()

		newSubjectServer(testutils.NewFakeSubjectStore()).routes().ServeHTTP(response, subjectRequest(http.MethodGet, "/subjects/not-an-id", ""))

		testutils.AssertStatusCode(t, response.Code, http.StatusNotFound)
	})

	t.Run("returns 500 when store fails", func(t *testing.T) {
		store := &subjectStoreStub{getSubject: func(context.Context, string, int64) (*data.Subject, error) { return nil, errors.New("boom") }}
		response := httptest.NewRecorder()

		newSubjectServer(store).routes().ServeHTTP(response, subjectRequest(http.MethodGet, "/subjects/7", ""))

		testutils.AssertStatusCode(t, response.Code, http.StatusInternalServerError)
	})
}

func TestCreateSubjectHandler(t *testing.T) {
	t.Run("returns 201 with created subject", func(t *testing.T) {
		store := testutils.NewFakeSubjectStore()
		response := httptest.NewRecorder()

		newSubjectServer(store).routes().ServeHTTP(response, subjectRequest(http.MethodPost, "/subjects", `{"subject_name":"coding"}`))

		testutils.AssertStatusCode(t, response.Code, http.StatusCreated)
		testutils.AssertContentType(t, response, jsonContentType)
		var body struct {
			Subject subjectResponse `json:"subject"`
		}
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		testutils.AssertCorrect(t, body.Subject.SubjectID, int64(1))
		testutils.AssertCorrect(t, body.Subject.SubjectName, "coding")
	})

	t.Run("returns 400 for malformed JSON", func(t *testing.T) {
		response := httptest.NewRecorder()

		newSubjectServer(testutils.NewFakeSubjectStore()).routes().ServeHTTP(response, subjectRequest(http.MethodPost, "/subjects", `{"subject_name":`))

		testutils.AssertStatusCode(t, response.Code, http.StatusBadRequest)
	})

	t.Run("returns 422 without persisting invalid subject", func(t *testing.T) {
		store := &subjectStoreStub{}
		response := httptest.NewRecorder()

		newSubjectServer(store).routes().ServeHTTP(response, subjectRequest(http.MethodPost, "/subjects", `{"subject_name":" "}`))

		testutils.AssertStatusCode(t, response.Code, http.StatusUnprocessableEntity)
		testutils.AssertCorrect(t, store.createCalls, 0)
		if !strings.Contains(response.Body.String(), `"subject_name"`) {
			t.Fatalf("got response body %s", response.Body.String())
		}
	})

	t.Run("returns 409 for duplicate subject", func(t *testing.T) {
		store := &subjectStoreStub{createSubject: func(context.Context, *data.Subject) (*data.Subject, error) { return nil, data.ErrDuplicateRecord }}
		response := httptest.NewRecorder()

		newSubjectServer(store).routes().ServeHTTP(response, subjectRequest(http.MethodPost, "/subjects", `{"subject_name":"coding"}`))

		testutils.AssertStatusCode(t, response.Code, http.StatusConflict)
		if !strings.Contains(response.Body.String(), "coding already exists") {
			t.Fatalf("got response body %s", response.Body.String())
		}
	})

	t.Run("returns 500 when store fails", func(t *testing.T) {
		store := &subjectStoreStub{createSubject: func(context.Context, *data.Subject) (*data.Subject, error) { return nil, errors.New("boom") }}
		response := httptest.NewRecorder()

		newSubjectServer(store).routes().ServeHTTP(response, subjectRequest(http.MethodPost, "/subjects", `{"subject_name":"coding"}`))

		testutils.AssertStatusCode(t, response.Code, http.StatusInternalServerError)
	})
}
