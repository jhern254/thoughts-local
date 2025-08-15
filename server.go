// server.go 
package main

import (
    "fmt"
    "os"
    "net/http"
//    "errors"
    "strings"

    "github.com/rs/zerolog" 
)

//const (
//    ErrWordNotFound     = DictionaryErr("cannot find word you searched for")
//    ErrWordExists       = DictionaryErr("cannot add word, it already exists")
//    ErrWordDoesNotExist = DictionaryErr("cannot update as word doesn't exist")
//)
//
//type Dictionary map[string]string
//type DictionaryErr string
//
//func (e DictionaryErr) Error() string {
//    return string(e)
//}

//var ErrInsufficientFunds = errors.New("cannot withdraw, insufficient funds")
//type Wallet struct {
//    balance Bitcoin
//}
//
//func (w *Wallet) Balance() Bitcoin {
//    return w.balance    // NOTE: same as *w dereference
//}
//
//func (w *Wallet) Withdraw(amount Bitcoin) error {
//    if amount > w.balance {
//        return fmt.Errorf("withdrawal failed: %w", ErrInsufficientFunds)
//    }
//    w.balance -= amount
//    return nil
//}
//
//// stdlib to print bitcoin
//type Stringer interface {
//    String() string 
//}
//
//func (b Bitcoin) String() string {
//    return fmt.Sprintf("%d BTC", b)
//}


// Public methods
type ThoughtStore interface {
    GetThought(subject string) string
}

type ThoughtServer struct {
    store ThoughtStore
}

// method needs pointer as input
func (s *ThoughtServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    subject := strings.ToLower(strings.TrimPrefix(r.URL.Path, "/subjects/"))
    w.WriteHeader(http.StatusNotFound)
    fmt.Fprint(w, s.store.GetThought(subject))
}

func GetThought(subject string) string {
    if subject == "coding" {
        return "I'm learning go!"
        
    }
    if subject == "ai" {
        return "agi 2025!"
    }
    return ""
}



func Run() {
    // configure package zerolog
    output := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: "2006-01-02 15:04:05"}
    logger := zerolog.New(output).With().
        Timestamp().
        Caller().   // adds file and line number
        Logger().Level(zerolog.ErrorLevel) // set log level 

    logger.Debug().Msg("Program ended.")
}

