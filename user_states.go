// user_states.go 
package main

import (
    "fmt"
    "io"
//    "os"
//    "net/http"
//    "errors"
//    "strings"
    "encoding/json"
)

type UserState struct {
    UserID  string
    Subjects []Subject
}

type UserStates []UserState

func NewUserStates(rdr io.Reader) (UserStates, error){
    var users UserStates
    if err := json.NewDecoder(rdr).Decode(&users); err != nil {
        return nil, fmt.Errorf("problem parsing user states: %w", err)
    }
    return users, nil
}


