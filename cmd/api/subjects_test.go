// subjects_test.go 
// handler logic
package main

import (
    "testing"
    "net/http"
    "net/http/httptest"
//    "fmt"
    "strings"
    "os"
    "encoding/json"
    "time"
//    "io"
    "github.com/jhern254/go-thoughts/internal/testutils"
    "github.com/rs/zerolog" 
)


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

// TODO: implement
func TestGETSubject(t *testing.T) {
    store := StubThoughtStore{
        map[string][]string{
            "coding": {"I'm learning go!"},
            "ai": {"agi 2025!"},
        },
        nil,
    }
    server := NewApplication(&store, config{}, zerolog.New(os.Stdout))

    // test return ok
    t.Run("returns 200 on subject name", func(t *testing.T) {
        request :=  newGetSubjectRequest("coding")
        response := httptest.NewRecorder()

        server.routes().ServeHTTP(response, request)

            // shape that matches your handler’s envelope
        var got struct {
            Subject struct {
                SubjectID int64     `json:"subject_id"`
                UserID    string    `json:"user_id"`
                Name      string    `json:"name"`
                CreatedAt time.Time `json:"created_at"`
                UpdatedAt time.Time `json:"updated_at"`
                Thoughts  []string  `json:"thoughts,omitempty"` // if you include it temporarily
            } `json:"subject"`
        }
        if err := json.NewDecoder(response.Body).Decode(&got); err != nil {
            t.Fatalf("decode: %v", err)
        }
        testutils.AssertCorrect(t, response.Code, http.StatusOK)
        testutils.AssertCorrect(t, got.Subject.Name, "coding")

    })

}

func TestGETThoughts(t *testing.T) {
    store := StubThoughtStore{
        map[string][]string{
            "coding": {"I'm learning go!"},
            "ai": {"agi 2025!"},
        },
        nil,
    }
    server := NewApplication(&store, config{}, zerolog.New(os.Stdout))

    t.Run("returns coding thoughts", func(t *testing.T) {
        request := newGetThoughtRequest("coding")
        response := httptest.NewRecorder()

        server.routes().ServeHTTP(response, request)

        testutils.AssertCorrect(t, response.Code, http.StatusOK)
        testutils.AssertCorrect(t, response.Body.String(), "I'm learning go!")
    })
    t.Run("return ai thoughts", func(t *testing.T) {
        request := newGetThoughtRequest("ai")
        response := httptest.NewRecorder()

        server.routes().ServeHTTP(response, request)

        testutils.AssertCorrect(t, response.Code, http.StatusOK)
        testutils.AssertCorrect(t, response.Body.String(), "agi 2025!")
    })
    t.Run("return 404 on missing subject", func(t *testing.T) {
        request := newGetThoughtRequest("physics")
        response := httptest.NewRecorder()

        server.routes().ServeHTTP(response, request)

        testutils.AssertCorrect(t, response.Code, http.StatusNotFound)
    })
}

func TestStoreThoughts(t *testing.T) {
    store := StubThoughtStore{
        map[string][]string{},
        nil,
    }
    server := NewApplication(&store, config{}, zerolog.New(os.Stdout))

    t.Run("it returns accepted on POST", func(t *testing.T) {
        subj := "coding"
        th := "I'm learning go!"
        request := newPostThoughtRequest(subj, th)
        response := httptest.NewRecorder()

        server.routes().ServeHTTP(response, request)
        testutils.AssertCorrect(t, response.Code, http.StatusAccepted)
//        assertCorrect(t, store.Count(), 1)
//        fmt.Printf("store is %+v", store)
        if len(store.subjectCalls) != 1 {
            t.Errorf("got %d calls to CaptureThought() want %d", len(store.subjectCalls), 1)
        }

        if store.subjectCalls[0] != subj {
            t.Errorf("did not store correct subj got %q want %q", store.subjectCalls[0], subj)
        }

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
    req.Header.Set("Content-Type", "text/plain; charset=utf-8")
    return req
}

