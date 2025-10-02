// subjects_test.go 
// handler logic
package main

import (
    "testing"
    "net/http"
    "net/http/httptest"
    "fmt"
    "strings"
    "os"
    "encoding/json"
    "time"
//    "io"
    "context"

    "github.com/jhern254/go-thoughts/internal/testutils"
    "github.com/jhern254/go-thoughts/internal/data"
    "github.com/rs/zerolog" 
)

type subjectEnvelope struct {
    Subject subjectThoughtResponse `json:"subject"`
}

// per test fn stub
type StoreStub struct {
    // subjects
    getSubjectFunc  func(ctx context.Context, userID, subject string) (*data.Subject, error)
    captureSubjectFunc func(ctx context.Context, userID, subject string) (int64, error)
    // thoughts
    // TODO: refactor to add ctx, err
    getThoughtsFunc func(userID, subject string) []string
    captureThoughtFunc func(userID, subject, thought string)
}

func (s StoreStub) GetSubject(ctx context.Context, userID, subject string) (*data.Subject, error) {
    if s.getSubjectFunc == nil { panic("GetSubject not stubbed") } 
    return s.getSubjectFunc(ctx, userID, subject)
}

func (s StoreStub) CaptureSubject(ctx context.Context, userID, subject string) (int64, error) {
    if s.captureSubjectFunc == nil { panic("CaptureSubject not stubbed") } 
    return s.captureSubjectFunc(ctx, userID, subject)
}

func (s StoreStub) GetThoughts(userID, subject string) []string {
    if s.getThoughtsFunc == nil { panic("GetThoughts not stubbed") } 
    return s.getThoughtsFunc(userID, subject)
}

func (s StoreStub) CaptureThought(userID, subject, thought string) {
    if s.captureThoughtFunc == nil { panic("CaptureThoughts not stubbed") } 
    // NOTE: temp, has no return value
    s.captureThoughtFunc(userID, subject, thought)
}


// NOTE: old code, temp wrapped for churn
type StubThoughtStore struct {
    thoughts map[string][]string
    subjectCalls    []string       // spy
}

// TODO: add userID impl 
func (s *StubThoughtStore) GetThoughts(_, subject string) []string {
    return s.thoughts[subject]
}

// TODO: add userID impl 
func (s *StubThoughtStore) CaptureThought(userID, subject, thought string) {
    s.thoughts[subject] = append(s.thoughts[subject], thought)            
    s.subjectCalls = append(s.subjectCalls, subject)
}

func (s *StubThoughtStore) Count() int {
    return len(s.thoughts)
}

type thoughtsAdapter struct { *StubThoughtStore }

// Provide no-op subject methods to satisfy the composite:
func (a thoughtsAdapter) GetSubject(ctx context.Context, u, n string) (*data.Subject, error) {
    return nil, data.ErrRecordNotFound
}

func (a thoughtsAdapter) CaptureSubject(ctx context.Context, u, n string) (int64, error) {
    return 0, nil
}

func TestGETSubject(t *testing.T) {
    // old stub that passed test
//    store := StubThoughtStore{
//        map[string][]string{
//            "coding": {"I'm learning go!"},
//            "ai": {"agi 2025!"},
//        },
//        nil,
//    }
    st := StoreStub{
        getSubjectFunc: func(ctx context.Context, uid, name string) (*data.Subject, error) {
            // GET handler needs a subject to exist:
            if name != "coding" { return nil, data.ErrRecordNotFound }  // for 404 test
            now := time.Unix(0, 0).UTC()
            return &data.Subject{
                SubjectID: 1,   // temp
                UserID: uid, 
                SubjectName: "coding",
                CreatedAt: now, 
                UpdatedAt: now,
                Thoughts:  nil, 
            }, nil
        },
    }
    server := NewApplication(st, config{}, zerolog.New(os.Stdout))

    t.Run("returns 200 on subject name", func(t *testing.T) {
        request :=  newGetSubjectRequest("coding")
        response := httptest.NewRecorder()

        server.routes().ServeHTTP(response, request)

        // shape that matches envelope
        var got struct {
            Subject struct {
                SubjectID int64     `json:"subject_id"`
                UserID    string    `json:"user_id"`
                SubjectName string  `json:"subject_name"`
                CreatedAt time.Time `json:"created_at"`
                UpdatedAt time.Time `json:"updated_at"`
                Thoughts  []string  `json:"thoughts,omitempty"` // if you include it temporarily
            } `json:"subject"`
        }

        if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
            t.Fatalf("decode error: %v", err)
        }
        testutils.AssertCorrect(t, response.Code, http.StatusOK)
        testutils.AssertCorrect(t, got.Subject.SubjectName, "coding")
    })
    t.Run("return 404 on missing subject", func(t *testing.T) {
        request := newGetSubjectRequest("physics")
        response := httptest.NewRecorder()

        server.routes().ServeHTTP(response, request)

        testutils.AssertCorrect(t, response.Code, http.StatusNotFound)
    })
}

//func TestPOSTSubject(t *testing.T) {
//    store := StubThoughtStore{
//        map[string][]string{
//            "coding": {"I'm learning go!"},
//            "ai": {"agi 2025!"},
//        },
//        nil,
//    }
//    server := NewApplication(&store, config{}, zerolog.New(os.Stdout))
//
//    t.Run("it returns accepted subject on POST", func(t *testing.T) {
//        subj := "coding"
//        request := newPostSubjectRequest(subj)
//        response := httptest.NewRecorder()
//
//        server.routes().ServeHTTP(response, request)
//        testutils.AssertCorrect(t, response.Code, http.StatusAccepted)
//
//        //TODO: implement red test here
//    })
//
//}


func TestGETThoughts(t *testing.T) {
    store := StubThoughtStore{
        map[string][]string{
            "coding": {"I'm learning go!"},
            "ai": {"agi 2025!"},
        },
        nil,
    }
    server := NewApplication(thoughtsAdapter{&store}, config{}, zerolog.New(os.Stdout))

    t.Run("returns coding thoughts", func(t *testing.T) {
        request := newGetThoughtRequest("coding")
        response := httptest.NewRecorder()

        server.routes().ServeHTTP(response, request)

        testutils.AssertCorrect(t, response.Code, http.StatusOK)
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

        testutils.AssertCorrect(t, response.Code, http.StatusOK)
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

        testutils.AssertCorrect(t, response.Code, http.StatusNotFound)
    })
}



// Refactor to JSON
func TestStoreThoughts(t *testing.T) {
    store := StubThoughtStore{
        map[string][]string{},
        nil,
    }
    server := NewApplication(thoughtsAdapter{&store}, config{}, zerolog.New(os.Stdout))

    t.Run("it returns accepted thoughts on POST", func(t *testing.T) {
        subj := "coding"
        th := "I'm learning go!"
        request := newPostThoughtRequest(subj, th)
        response := httptest.NewRecorder()

        server.routes().ServeHTTP(response, request)
        testutils.AssertCorrect(t, response.Code, http.StatusAccepted)
//        assertCorrect(t, store.Count(), 1)
//        fmt.Printf("store is %+v", store)

        // fix
//        if len(store.subjectCalls) != 1 {
//            t.Errorf("got %d calls to CaptureThought() want %d", len(store.subjectCalls), 1)
//        }
//
//        if store.subjectCalls[0] != subj {
//            t.Errorf("did not store correct subj got %q want %q", store.subjectCalls[0], subj)
//        }

    })
}

// helper fns
func newGetSubjectRequest(subject string) *http.Request {
    return httptest.NewRequest(
        http.MethodGet,
        "/subjects/"+subject,
        nil,
    )
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
    payload := fmt.Sprintf(`{"subject_name":%q}`, subject)
    req := httptest.NewRequest(
        http.MethodPost,
        "/subjects",
        strings.NewReader(payload),
    )
    req.Header.Set("Content-Type", jsonContentType)
    return req
}

