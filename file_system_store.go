// file_system_store.go 
package main

import (
//    "fmt"
    "io"
//    "os"
//    "net/http"
//    "errors"
//    "strings"
//    "encoding/json"
)

type FileSystemThoughtStore struct {
    database io.Reader
}

// GetAllUserStates is GetLeague() []Players equivalent
func (f *FileSystemThoughtStore) GetAllUserStates() UserStates {
    users, _ := NewUserStates(f.database) 
    return users
}

func (f *FileSystemThoughtStore) GetUserState(userID string) UserState {
    for _, u := range f.GetAllUserStates() {
        if u.UserID == userID {
            return u
        }
    }
    return UserState{}
}


