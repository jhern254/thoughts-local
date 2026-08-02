package testutils

import (
	"errors"
	"net/http/httptest"
	"reflect"
	"testing"
)

// generic
func AssertCorrect[T comparable](t testing.TB, got, want T) {
	// helper fn, always use testing.TB
	t.Helper()
	//    got := wallet.Balance()
	if got != want {
		t.Errorf("\ngot %+v, \nwant %+v", got, want)
	}
}

// generic using reflect, not type safe
func AssertCorrectStruct[T any](t testing.TB, got, want T) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("\ngot  %+v,\nwant %+v", got, want)
	}
}

func AssertContentType(t testing.TB, response *httptest.ResponseRecorder, want string) {
	t.Helper()
	if response.Result().Header.Get("content-type") != want {
		t.Errorf("response did not have content-type of got %s, want %v", want, response.Result().Header)
	}
}

func AssertStatusCode(t *testing.T, got, want int) {
	t.Helper()
	if got != want {
		t.Errorf("wrong status code: got %d, want %d", got, want)
	}
}

func AssertNoError(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Error("got error but didn't want one")
	}
}

func AssertError(t testing.TB, err, want error) {
	t.Helper()
	if err == nil {
		t.Error("wanted error but didn't get one")
	}

	if !errors.Is(err, want) {
		t.Errorf("\ngot %s \nwant %s", err, want)
	}
}
