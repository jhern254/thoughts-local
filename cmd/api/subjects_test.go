// subjects_test.go
// handler logic
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	//    "fmt"
	"strings"
	//    "os"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/testutils"
	"github.com/julienschmidt/httprouter"
	"github.com/rs/zerolog"
)

type subjectEnvelope struct {
	Subject subjectThoughtResponse `json:"subject"`
}

// TODO: refactor to domain response
type thoughtEnvelope struct {
	Thought struct {
		ThoughtID int64  `json:"thought_id"`
		UserID    string `json:"user_id"`
		SubjectID int64  `json:"subject_id"`
		EventID   int64  `json:"event_id"`
		Thought   string `json:"thought"`
		CreatedAt string `json:"created_at"`
		UpdatedAt string `json:"-"`
	} `json:"thought"`
}

// per test fn stub
type StoreStub struct {
	// subjects
	getSubjectFunc     func(ctx context.Context, userID, subject string) (*data.Subject, error)
	captureSubjectFunc func(ctx context.Context, userID string, subject *data.Subject) (int64, error)
	// thoughts
	// TODO: refactor to add ctx, err
	getThoughtsFunc    func(userID, subject string) []string
	captureThoughtFunc func(ctx context.Context, userID, subject, thought string) (int64, error)
}

func (s StoreStub) GetSubject(ctx context.Context, userID, subject string) (*data.Subject, error) {
	if s.getSubjectFunc == nil {
		panic("GetSubject not stubbed")
	}
	return s.getSubjectFunc(ctx, userID, subject)
}

func (s StoreStub) CaptureSubject(ctx context.Context, userID string, subject *data.Subject) (int64, error) {
	if s.captureSubjectFunc == nil {
		panic("CaptureSubject not stubbed")
	}
	return s.captureSubjectFunc(ctx, userID, subject)
}

func (s StoreStub) GetThoughts(userID, subject string) []string {
	if s.getThoughtsFunc == nil {
		panic("GetThoughts not stubbed")
	}
	return s.getThoughtsFunc(userID, subject)
}

func (s StoreStub) CaptureThought(ctx context.Context, userID, subject, thought string) (int64, error) {
	if s.captureThoughtFunc == nil {
		panic("CaptureThoughts not stubbed")
	}
	// NOTE: temp, has no return value
	return s.captureThoughtFunc(ctx, userID, subject, thought)
}

// NOTE: old code, temp wrapped for churn
type StubThoughtStore struct {
	thoughts     map[string][]string
	subjectCalls []string // spy
}

// TODO: add userID impl
func (s *StubThoughtStore) GetThoughts(_, subject string) []string {
	return s.thoughts[subject]
}

// TODO: add userID impl
func (s *StubThoughtStore) CaptureThought(ctx context.Context, userID, subject, thought string) (int64, error) {
	if s.thoughts == nil {
		s.thoughts = make(map[string][]string)
	}
	s.thoughts[subject] = append(s.thoughts[subject], thought)
	s.subjectCalls = append(s.subjectCalls, subject)

	newID := int64(len(s.thoughts[subject])) // 1-based ID
	return newID, nil
}

func (s *StubThoughtStore) Count() int {
	return len(s.thoughts)
}

// TODO: refactor out
type thoughtsAdapter struct{ *StubThoughtStore }

// dummy
func (a thoughtsAdapter) GetSubject(ctx context.Context, u, n string) (*data.Subject, error) {
	return nil, data.ErrRecordNotFound
}

func (a thoughtsAdapter) CaptureSubject(ctx context.Context, u string, s *data.Subject) (int64, error) {
	return 0, nil
}

// tests
func TestGETSubject(t *testing.T) {
	st := StoreStub{
		getSubjectFunc: func(ctx context.Context, uid, name string) (*data.Subject, error) {
			// GET handler needs a subject to exist:
			if name != "coding" {
				return nil, data.ErrRecordNotFound
			} // for 404 test
			now := time.Unix(0, 0).UTC()
			return &data.Subject{
				SubjectID:   1, // temp
				UserID:      uid,
				SubjectName: "coding",
				CreatedAt:   now,
				UpdatedAt:   now,
				Thoughts:    nil,
			}, nil
		},
	}
	server := NewApplication(st, config{}, zerolog.New(io.Discard))

	t.Run("returns 200 on subject name", func(t *testing.T) {
		request := newGetSubjectRequest("coding")
		response := httptest.NewRecorder()

		server.routes().ServeHTTP(response, request)

		// shape that matches envelope
		var got struct {
			Subject struct {
				SubjectID   int64     `json:"subject_id"`
				UserID      string    `json:"user_id"`
				SubjectName string    `json:"subject_name"`
				CreatedAt   time.Time `json:"created_at"`
				UpdatedAt   time.Time `json:"updated_at"`
				Thoughts    []string  `json:"thoughts,omitempty"` // if you include it temporarily
			} `json:"subject"`
		}

		if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
			t.Fatalf("decode error: %v", err)
		}
		testutils.AssertStatusCode(t, response.Code, http.StatusOK)
		testutils.AssertCorrect(t, got.Subject.SubjectName, "coding")
	})
	t.Run("return 404 on missing subject", func(t *testing.T) {
		request := newGetSubjectRequest("physics")
		response := httptest.NewRecorder()

		server.routes().ServeHTTP(response, request)

		testutils.AssertStatusCode(t, response.Code, http.StatusNotFound)
	})
	t.Run("return 500 when store returns generic error", func(t *testing.T) {
		st = StoreStub{
			getSubjectFunc: func(ctx context.Context, uid, name string) (*data.Subject, error) {
				return nil, errors.New("boom")
			},
		}
		server := NewApplication(st, config{}, zerolog.New(io.Discard))

		response := httptest.NewRecorder()
		server.routes().ServeHTTP(response, newGetSubjectRequest("coding"))

		testutils.AssertStatusCode(t, response.Code, http.StatusInternalServerError)
	})
}

// TODO: understand
func TestGETSubject_ContextAndErrors(t *testing.T) {
	t.Run("context is propagated: canceled => handler returns 500 (current behavior)", func(t *testing.T) {
		// Stub that observes ctx and returns ctx.Err()
		st := StoreStub{
			getSubjectFunc: func(ctx context.Context, uid, name string) (*data.Subject, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
		}
		server := NewApplication(st, config{}, zerolog.New(io.Discard))

		// Build request with a cancelable context and cancel it before serving.
		request := newGetSubjectRequest("coding")
		ctx, cancel := context.WithCancel(request.Context())
		cancel() // simulate client disconnect or upstream cancel
		request = request.WithContext(ctx)

		response := httptest.NewRecorder()
		server.routes().ServeHTTP(response, request)

		// With your current helpers, any non-ErrRecordNotFound error maps to 500.
		// If you later special-case context errors, adjust the expectation (e.g., 499/504).
		testutils.AssertStatusCode(t, response.Code, http.StatusInternalServerError)
	})

	t.Run("context is propagated: deadline exceeded => 500 (current behavior)", func(t *testing.T) {
		st := StoreStub{
			getSubjectFunc: func(ctx context.Context, uid, name string) (*data.Subject, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			},
		}
		server := NewApplication(st, config{}, zerolog.New(io.Discard))

		request := newGetSubjectRequest("coding")
		ctx, cancel := context.WithTimeout(request.Context(), 1*time.Nanosecond)
		defer cancel()
		// Give it a tick so the deadline trips.
		time.Sleep(2 * time.Nanosecond)
		request = request.WithContext(ctx)

		response := httptest.NewRecorder()
		server.routes().ServeHTTP(response, request)

		testutils.AssertStatusCode(t, response.Code, http.StatusInternalServerError)
	})
}

func TestPOSTSubject(t *testing.T) {
	st := StoreStub{
		captureSubjectFunc: func(ctx context.Context, uid string, subj *data.Subject) (int64, error) {
			// assert handler mapped DTO -> domain correctly before hitting store.
			//            if uid != "test-user" { t.Fatalf("userID: got %q", uid) }
			if subj.SubjectName != "coding" {
				t.Fatalf("subject_name: got %q", subj.SubjectName)
			}
			// return a fake DB id.
			return 123, nil
		},
	}
	server := NewApplication(&st, config{}, zerolog.New(io.Discard))

	t.Run("returns created 201 subject on POST", func(t *testing.T) {
		subj := "coding"
		request := newPostSubjectRequest(subj)
		response := httptest.NewRecorder()

		server.routes().ServeHTTP(response, request)
		// NOTE: need if checking logs
		//        t.Logf("body: %s", response.Body.String())
		testutils.AssertStatusCode(t, response.Code, http.StatusCreated)
		testutils.AssertContentType(t, response, jsonContentType)

		// GET to assert
		// decode JSON body
		var env subjectEnvelope
		if err := json.NewDecoder(response.Body).Decode(&env); err != nil {
			t.Fatalf("decode json: %v\nbody:\n%s", err, response.Body.String())
		}
		got := env.Subject
		testutils.AssertCorrect(t, got.SubjectName, "coding")
		testutils.AssertCorrect(t, got.SubjectID, 123)
	})
	t.Run("returns 422 without persisting invalid subject", func(t *testing.T) {
		storeCalls := 0
		st := StoreStub{
			captureSubjectFunc: func(ctx context.Context, uid string, subj *data.Subject) (int64, error) {
				storeCalls++
				return 123, nil
			},
		}
		server := NewApplication(st, config{}, zerolog.New(io.Discard))

		response := httptest.NewRecorder()
		server.routes().ServeHTTP(response, newPostSubjectRequest(" "))

		testutils.AssertStatusCode(t, response.Code, http.StatusUnprocessableEntity)
		testutils.AssertCorrect(t, storeCalls, 0)
	})
	t.Run("returns 409 when duplicate record", func(t *testing.T) {
		st := StoreStub{
			captureSubjectFunc: func(ctx context.Context, uid string, subj *data.Subject) (int64, error) {
				return 0, data.ErrDuplicateRecord
			},
		}
		server := NewApplication(st, config{}, zerolog.New(io.Discard))

		response := httptest.NewRecorder()
		server.routes().ServeHTTP(response, newPostSubjectRequest("coding"))

		testutils.AssertStatusCode(t, response.Code, http.StatusConflict)
	})
	t.Run("returns 500 when store returns generic error", func(t *testing.T) {
		st := StoreStub{
			captureSubjectFunc: func(ctx context.Context, uid string, subj *data.Subject) (int64, error) {
				return 0, errors.New("boom")
			},
		}
		server := NewApplication(st, config{}, zerolog.New(io.Discard))

		response := httptest.NewRecorder()
		server.routes().ServeHTTP(response, newPostSubjectRequest("coding"))

		testutils.AssertStatusCode(t, response.Code, http.StatusInternalServerError)
	})
}

// TODO: test context and err for POST subject

func TestGETThoughts(t *testing.T) {
	store := StubThoughtStore{
		map[string][]string{
			"coding": {"I'm learning go!"},
			"ai":     {"agi 2025!"},
		},
		nil,
	}
	server := NewApplication(thoughtsAdapter{&store}, config{}, zerolog.New(io.Discard))

	t.Run("returns coding thoughts", func(t *testing.T) {
		request := newGetThoughtRequest("coding")
		response := httptest.NewRecorder()

		server.routes().ServeHTTP(response, request)

		testutils.AssertStatusCode(t, response.Code, http.StatusOK)
		testutils.AssertContentType(t, response, jsonContentType)

		// decode JSON body
		var env subjectEnvelope
		if err := json.NewDecoder(response.Body).Decode(&env); err != nil {
			t.Fatalf("decode json: %v\nbody:\n%s", err, response.Body.String())
		}
		got := env.Subject

		testutils.AssertCorrect(t, got.SubjectName, "coding")
		testutils.AssertCorrectStruct(t, got.Thoughts, []string{"I'm learning go!"})
	})
	t.Run("return ai thoughts", func(t *testing.T) {
		request := newGetThoughtRequest("ai")
		response := httptest.NewRecorder()

		server.routes().ServeHTTP(response, request)

		testutils.AssertStatusCode(t, response.Code, http.StatusOK)
		testutils.AssertContentType(t, response, jsonContentType)

		// decode JSON body
		var env subjectEnvelope
		if err := json.NewDecoder(response.Body).Decode(&env); err != nil {
			t.Fatalf("decode json: %v\nbody:\n%s", err, response.Body.String())
		}
		got := env.Subject

		testutils.AssertCorrect(t, got.SubjectName, "ai")
		testutils.AssertCorrectStruct(t, got.Thoughts, []string{"agi 2025!"})
	})
	t.Run("return 404 on missing subject", func(t *testing.T) {
		request := newGetThoughtRequest("physics")
		response := httptest.NewRecorder()

		server.routes().ServeHTTP(response, request)

		testutils.AssertStatusCode(t, response.Code, http.StatusNotFound)
	})
}

// Refactor to JSON
func TestPOSTThoughts(t *testing.T) {
	//    store := StubThoughtStore{
	//        map[string][]string{},
	//        nil,
	//    }
	store := StoreStub{
		captureThoughtFunc: func(ctx context.Context, userID, subject, thought string) (int64, error) {
			// return a fake DB id.
			return 123, nil
		},
	}
	// TODO: not sure if correct
	//    server := NewApplication(thoughtsAdapter{&store}, config{}, zerolog.New(io.Discard))
	server := NewApplication(store, config{}, zerolog.New(io.Discard))

	t.Run("returns created 201 thoughts on POST", func(t *testing.T) {
		subj := "coding"
		th := "I'm learning go!"
		request := newPostThoughtRequest(subj, th)
		response := httptest.NewRecorder()

		server.routes().ServeHTTP(response, request)
		testutils.AssertStatusCode(t, response.Code, http.StatusCreated)
		testutils.AssertContentType(t, response, jsonContentType)

		// GET to assert
		// decode JSON body
		var env thoughtEnvelope
		if err := json.NewDecoder(response.Body).Decode(&env); err != nil {
			t.Fatalf("decode json: %v\nbody:\n%s", err, response.Body.String())
		}
		got := env.Thought
		testutils.AssertCorrect(t, got.Thought, "I'm learning go!")
		testutils.AssertCorrect(t, got.ThoughtID, 123)
	})
	t.Run("returns 500 when store returns generic error", func(t *testing.T) {
		st := StoreStub{
			captureThoughtFunc: func(ctx context.Context, userID, subject, thought string) (int64, error) {
				return 0, errors.New("boom")
			},
		}
		server := NewApplication(st, config{}, zerolog.New(io.Discard))

		response := httptest.NewRecorder()
		server.routes().ServeHTTP(response, newPostThoughtRequest("coding", "hello world"))

		testutils.AssertStatusCode(t, response.Code, http.StatusInternalServerError)
	})
}

// helper fns
func newGetSubjectRequest(subject string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/subjects/"+subject, nil)
	// attach httprouter params like a router would
	params := httprouter.Params{httprouter.Param{Key: "subject", Value: subject}}
	ctx := context.WithValue(req.Context(), httprouter.ParamsKey, params)
	return req.WithContext(ctx)
}

func newGetThoughtRequest(subject string) *http.Request {
	return httptest.NewRequest(
		http.MethodGet,
		"/subjects/"+subject+"/thoughts",
		nil,
	)
}

func newPostThoughtRequest(subject, thought string) *http.Request {
	//    req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/subjects/%s/thoughts", subject), strings.NewReader(thought))
	//    return req
	req := httptest.NewRequest(
		http.MethodPost,
		"/subjects/"+subject+"/thoughts",
		strings.NewReader(thought),
	)
	// TODO: is this a bug?
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	return req
}

func newPostSubjectRequest(subject string) *http.Request {
	payload := `{"subject_name":"` + subject + `"}`
	req := httptest.NewRequest(
		http.MethodPost,
		"/subjects",
		strings.NewReader(payload),
	)
	req.Header.Set("Content-Type", jsonContentType)
	return req
}
