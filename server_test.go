// server_test.go 
package main

import (
    "testing"
    "errors"
    "net/http"
    "net/http/httptest"
//    "reflect"
    "fmt"
    "strings"
)

type StubThoughtStore struct {
    thoughts map[string][]string
    subjectCalls    []string       // spy
}

func (s *StubThoughtStore) GetThoughts(subject string) []string {
    return s.thoughts[subject]
}

func (s *StubThoughtStore) CaptureThought(subject, thought string) {
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
    server := &ThoughtServer{&store}

    t.Run("returns coding thoughts", func(t *testing.T) {
        request := newGetThoughtRequest("coding")
        response := httptest.NewRecorder()

        server.ServeHTTP(response, request)

        assertCorrect(t, response.Code, http.StatusOK)
        assertCorrect(t, response.Body.String(), "I'm learning go!")
    })
    t.Run("return ai thoughts", func(t *testing.T) {
        request := newGetThoughtRequest("ai")
        response := httptest.NewRecorder()

        server.ServeHTTP(response, request)

        assertCorrect(t, response.Code, http.StatusOK)
        assertCorrect(t, response.Body.String(), "agi 2025!")
    })
    t.Run("return 404 on missing subject", func(t *testing.T) {
        request := newGetThoughtRequest("physics")
        response := httptest.NewRecorder()

        server.ServeHTTP(response, request)

        assertCorrect(t, response.Code, http.StatusNotFound)
    })
}

func TestStoreThoughts(t *testing.T) {
    store := StubThoughtStore{
        map[string][]string{},
        nil,
    }
    server := &ThoughtServer{&store}

    t.Run("it returns accepted on POST", func(t *testing.T) {
        request, _ := http.NewRequest(http.MethodPost, "/subjects/coding", strings.NewReader("I'm learning go!"))
        response := httptest.NewRecorder()

        server.ServeHTTP(response, request)
        assertCorrect(t, response.Code, http.StatusAccepted)
//        assertCorrect(t, store.Count(), 1)
//        fmt.Printf("store is %+v", store)
        if len(store.subjectCalls) != 1 {
            t.Errorf("got %d calls to CaptureThought() want %d", len(store.subjectCalls), 1)
        }
    })
}

func newGetThoughtRequest(subject string) *http.Request {
    req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/subjects/%s", subject), nil)
    return req
}

// generic
func assertCorrect[T comparable](t testing.TB, got, want T) {
    // helper fn, always use testing.TB
    t.Helper()  
//    got := wallet.Balance()
    if got != want {
        t.Errorf("\ngot %+v, \nwant %+v", got, want)
    }
}


// generic using reflect, not type safe
//func assertCorrect[T any](t testing.TB, got, want T) {
//	t.Helper()
//	if !reflect.DeepEqual(got, want) {
//		t.Errorf("\ngot  %+v,\nwant %+v", got, want)
//	}
//}

func assertNoError(t testing.TB, err error) {
    t.Helper()
    if err != nil {
        t.Error("got error but didn't want one")
    }
}

func assertError(t testing.TB, err , want error) {
    t.Helper()
    if err == nil {
        t.Error("wanted error but didn't get one")
    }

    if !errors.Is(err, want) {
        t.Errorf("\ngot %s \nwant %s", err, want)
    }
}

