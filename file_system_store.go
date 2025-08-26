// file_system_store.go 
package main

import (
//    "fmt"
    "io"
//    "os"
//    "net/http"
//    "errors"
//    "strings"
    "encoding/json"
)

type FileSystemThoughtStore struct {
    database io.Reader
}

func (f *FileSystemThoughtStore) GetUserState(userID string) UserState {
    var users []UserState
    if err := json.NewDecoder(f.database).Decode(&users); err != nil {
        return UserState{} // handle errors properly later
    }

    for _, u := range users {
        if u.UserID == userID {
            return u
        }
    }
    return UserState{}
}


