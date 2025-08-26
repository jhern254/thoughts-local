// server_test.go 
package main

import (
    "testing"
    "errors"
    "net/http"
    "net/http/httptest"
    "reflect"
    "fmt"
    "strings"
    "encoding/json"
    "io"
)

type StubThoughtStore struct {
    thoughts map[string][]string
    subjectCalls    []string       // spy
    // NOTE: temporarily holds thoughts as well
    userState       UserState
}

func (s *StubThoughtStore) GetThoughts(subject string) []string {
    return s.thoughts[subject]
}

func (s *StubThoughtStore) CaptureThought(subject, thought string) {
    s.thoughts[subject] = append(s.thoughts[subject], thought)            
    s.subjectCalls = append(s.subjectCalls, subject)
}

func (s *StubThoughtStore) GetUserState(userID string) UserState {
    return s.userState
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
        UserState{},
    }
    server := NewThoughtServer(&store)

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
        UserState{},
    }
    server := NewThoughtServer(&store)

    t.Run("it returns accepted on POST", func(t *testing.T) {
        subj := "coding"
        th := "I'm learning go!"
        request := newPostThoughtRequest(subj, th)
        response := httptest.NewRecorder()

        server.ServeHTTP(response, request)
        assertCorrect(t, response.Code, http.StatusAccepted)
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

func TestUserState(t *testing.T) {
    store := StubThoughtStore{}
    server := NewThoughtServer(&store)

    t.Run("it returns a 200 on /users/{id}/state", func(t *testing.T) {
        request := newUserStateRequest("1")
        response := httptest.NewRecorder()

        server.ServeHTTP(response, request)

        var got UserState
        err := json.NewDecoder(response.Body).Decode(&got)
        if err != nil {
            t.Fatalf("Unable to parse response from server %q into UserState, '%v'", response.Body, err)
        }

        assertCorrect(t, response.Code, http.StatusOK)
    })
    t.Run("it returns thought userState as JSON", func(t *testing.T) {
        wantedState := UserState{
            UserID: "2",
            Subjects: []Subject{
                {"Physics", "Idk physics"},
                {"Code", "I'm learning go!"},
                {"AI", "Neural Networks work!"},
            },
        }

        store = StubThoughtStore{nil, nil, wantedState}
        server = NewThoughtServer(&store)

        request := newUserStateRequest("2")
        response := httptest.NewRecorder()
        
        server.ServeHTTP(response, request)

        // old way
//        var got UserState
//        err := json.NewDecoder(response.Body).Decode(&got)
        // injected store state JSON
        got := getUserStateFromResponse(t, response.Body)

        assertCorrect(t, response.Code, http.StatusOK)
        assertCorrectStruct(t, got, wantedState)

    })
}

// helper fns
func newGetThoughtRequest(subject string) *http.Request {
    req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/subjects/%s", subject), nil)
    return req
}

func newPostThoughtRequest(subject, thought string) *http.Request {
    req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/subjects/%s", subject), strings.NewReader(thought))
    return req
}

func newUserStateRequest(userID string) *http.Request {
    req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/users/%s/state", userID), nil)
    return req
}

// decode raw JSON userState from handler and return 
func getUserStateFromResponse(t testing.TB, body io.Reader) (userState UserState) {
    t.Helper()
    err := json.NewDecoder(body).Decode(&userState)
    if err != nil {
        t.Fatalf("Unable to parse response from server %q into UserState, '%v'", body, err)
    }

    return
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
func assertCorrectStruct[T any](t testing.TB, got, want T) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("\ngot  %+v,\nwant %+v", got, want)
	}
}

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

