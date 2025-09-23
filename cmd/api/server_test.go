// server_test.go 
package main

import (
    "testing"
    "net/http"
    "net/http/httptest"
    "fmt"
    "strings"
    "os"
//    "encoding/json"
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

func TestGETThoughts(t *testing.T) {
    store := StubThoughtStore{
        map[string][]string{
            "coding": {"I'm learning go!"},
            "ai": {"agi 2025!"},
        },
        nil,
    }
    server := NewThoughtServer(&store, config{}, zerolog.New(os.Stdout))

    t.Run("returns coding thoughts", func(t *testing.T) {
        request := newGetThoughtRequest("coding")
        response := httptest.NewRecorder()

        server.ServeHTTP(response, request)

        testutils.AssertCorrect(t, response.Code, http.StatusOK)
        testutils.AssertCorrect(t, response.Body.String(), "I'm learning go!")
    })
    t.Run("return ai thoughts", func(t *testing.T) {
        request := newGetThoughtRequest("ai")
        response := httptest.NewRecorder()

        server.ServeHTTP(response, request)

        testutils.AssertCorrect(t, response.Code, http.StatusOK)
        testutils.AssertCorrect(t, response.Body.String(), "agi 2025!")
    })
    t.Run("return 404 on missing subject", func(t *testing.T) {
        request := newGetThoughtRequest("physics")
        response := httptest.NewRecorder()

        server.ServeHTTP(response, request)

        testutils.AssertCorrect(t, response.Code, http.StatusNotFound)
    })
}

func TestStoreThoughts(t *testing.T) {
    store := StubThoughtStore{
        map[string][]string{},
        nil,
    }
    server := NewThoughtServer(&store, config{}, zerolog.New(os.Stdout))

    t.Run("it returns accepted on POST", func(t *testing.T) {
        subj := "coding"
        th := "I'm learning go!"
        request := newPostThoughtRequest(subj, th)
        response := httptest.NewRecorder()

        server.ServeHTTP(response, request)
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

//    t.Run("it returns thought userState as JSON", func(t *testing.T) {
//        wantedState := UserState{
//            UserID: "2",
//            Subjects: []Subject{
//                {"Physics", []string{"Idk physics"}, },
//                {"Code", []string{"I'm learning go!"}, },
//                {"AI", []string{"Neural Networks work!"}, },
//            },
//        }
//
//        store = StubThoughtStore{nil, nil, wantedState}
//        server = NewThoughtServer(&store)
//
//        request := newUserStateRequest("2")
//        response := httptest.NewRecorder()
//        
//        server.ServeHTTP(response, request)
//
//        // injected store state JSON
//        got := getUserStateFromResponse(t, response.Body)
//
//        assertCorrect(t, response.Code, http.StatusOK)
//        assertCorrectStruct(t, got, wantedState)
//        assertContentType(t, response, jsonContentType)
//    })

// helper fns
func newGetThoughtRequest(subject string) *http.Request {
    req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/subjects/%s", subject), nil)
    return req
}

func newPostThoughtRequest(subject, thought string) *http.Request {
    req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/subjects/%s", subject), strings.NewReader(thought))
    return req
}
