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
    database io.ReadWriteSeeker
}

// GetAllUserStates is GetLeague() []Players equivalent
func (f *FileSystemThoughtStore) GetAllUserStates() UserStates {
    f.database.Seek(0, 0)
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

func (f *FileSystemThoughtStore) GetThoughts(subject string) []string {
    for _, u := range f.GetAllUserStates() {
        for _, s := range u.Subjects {
            if s.Name == subject {
                // Return a copy so callers can't mutate the underlying slice.
                out := make([]string, len(s.Thoughts))
                copy(out, s.Thoughts)
                return out
            }
        }
    }
    return nil
}





