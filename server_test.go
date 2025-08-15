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

// NOTE: methods need same fn signature, else use anon inline fns in struct
//type walletOperation func(w *Wallet, amount Bitcoin) error

// table tests
//func TestWallet(t *testing.T) {
//
//    testCases := []struct {
//        name            string
//        initialBalance  Bitcoin
//        operation       walletOperation
//        amount          Bitcoin
//        wantError         error
//        wantBalance     Bitcoin
//    }{
//        {
// NOTE: can init anon structs(x struct, y string[]): struct {Name string}{"Jun"},[]string{"Jun"}
//            name:          "deposit bitcoin to wallet",
//            initialBalance: Bitcoin(0),
//            // NOTE: anon inline fn for custom 
//            operation:     func(w *Wallet, amount Bitcoin) error { w.Deposit(amount); return nil },
//            amount:        Bitcoin(10),
//            wantError:     nil,
//            wantBalance:   Bitcoin(10),
//        },
//        {
//            name:          "withdraw bitcoin from wallet",
//            initialBalance: Bitcoin(20),
//            operation:     (*Wallet).Withdraw,
//            amount:        Bitcoin(10),
//            wantError:     nil,
//            wantBalance:   Bitcoin(10),
//        },
//        {
//            name:          "withdraw insufficient funds",
//            initialBalance: Bitcoin(20),
//            operation:     (*Wallet).Withdraw,
//            amount:        Bitcoin(100),
//            wantError:     ErrInsufficientFunds,
//            wantBalance:   Bitcoin(20),
//        },
//    }
//
//    for _, tc := range testCases {
//        tc := tc // capture range variable to avoid issues in parallel tests
//        t.Run(tc.name, func(t *testing.T) {
//            wallet := Wallet{balance: tc.initialBalance}
//
//            err := tc.operation(&wallet, tc.amount)
//
//            if tc.wantError != nil {
//                assertError(t, err, tc.wantError)
//            } else {
//                assertNoError(t, err)
//            }
//
//            assertBalance(t, wallet, tc.wantBalance)
//        })
//    }
//
//}

func TestGETThoughts(t *testing.T) {
    t.Run("returns coding thoughts", func(t *testing.T) {
        request := newGetThoughtRequest("coding")
        response := httptest.NewRecorder()

        ThoughtServer(response, request)

        assertCorrect(t, response.Body.String(), "I'm learning go!")
    })
    t.Run("return ai thoughts", func(t *testing.T) {
        request := newGetThoughtRequest("ai")
        response := httptest.NewRecorder()

        ThoughtServer(response, request)

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

