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
    database io.ReadWriteSeeker
}

// GetAllUserStates is GetLeague() []Players equivalent
func (f *FileSystemThoughtStore) GetAllUserStates() UserStates {
    f.database.Seek(0, 0)
    // TODO: handle err
    users, _ := NewUserStates(f.database) 
//    fmt.Printf("users (+v): %+v", users)
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

// TODO: fix. base impl, still unsafe since not truncating
func (f *FileSystemThoughtStore) CaptureThought(subject, thought string) {
    users := f.GetAllUserStates()

    // 1) Update existing subject (first match wins)
    for ui := range users {
        for si := range users[ui].Subjects {
            if users[ui].Subjects[si].Name == subject {
                users[ui].Subjects[si].Thoughts = append(users[ui].Subjects[si].Thoughts, thought)
                // write at beginnning of file
                f.database.Seek(0, 0)
                _ = json.NewEncoder(f.database).Encode(users) // Tape comes later in the chapter
                // NOTE: doesn't handle multiple writes? 
                return
            }
        }
    }

    // 2) No subject found: append new subject to the first user (mirrors "append new Player")
    if len(users) > 0 {
        users[0].Subjects = append(users[0].Subjects, Subject{
            Name:     subject,
            Thoughts: []string{thought},
        })
        f.database.Seek(0, 0)
        _ = json.NewEncoder(f.database).Encode(users)
    }
}




