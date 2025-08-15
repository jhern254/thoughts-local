// server.go 
package server

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
func ThoughtServer(w http.ResponseWriter, r *http.Request) {
    subject := strings.TrimPrefix(r.URL.Path, "/subjects/")   

    if subject == "coding" {
        fmt.Fprint(w, "I'm learning go!")
        return
    }

    if subject == "ai" {
        fmt.Fprint(w, "agi 2025!")
        return
    }

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

