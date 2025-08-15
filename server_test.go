// server_test.go 
package server

import (
    "testing"
    "errors"
    "net/http"
    "net/http/httptest"
//    "reflect"
    "fmt"
)

func TestGETThoughts(t *testing.T) {
    server := &ThoughtServer{}

    t.Run("returns coding thoughts", func(t *testing.T) {
        request := newGetThoughtRequest("coding")
        response := httptest.NewRecorder()

        server.ServeHTTP(response, request)

        assertCorrect(t, response.Body.String(), "I'm learning go!")
    })
    t.Run("return ai thoughts", func(t *testing.T) {
        request := newGetThoughtRequest("ai")
        response := httptest.NewRecorder()

        server.ServeHTTP(response, request)

        assertCorrect(t, response.Body.String(), "agi 2025!")
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

