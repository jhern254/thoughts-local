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

// -- private DTOs only used for JSON <-> disk --
type subjectRow struct {
    Name     string   `json:"Name"`
    Thoughts []string `json:"Thoughts"`
}

type userRow struct {
    UserID   string      `json:"UserID"`
    Subjects []subjectRow `json:"Subjects"`
}

type dbFile []userRow

func (f *FileSystemThoughtStore) load() (dbFile, error) {
    if _, err := f.database.Seek(0, 0); err != nil {
        return nil, err
    }
    var db dbFile
    if err := json.NewDecoder(f.database).Decode(&db); err != nil {
        return nil, err
    }
    return db, nil
}

// GetAllUserStates is GetLeague() []Players equivalent
//func (f *FileSystemThoughtStore) GetAllUserStates() UserStates {
//    f.database.Seek(0, 0)
//    users, _ := NewUserStates(f.database) 
////    fmt.Printf("users (+v): %+v", users)
//    return users
//}

//func (f *FileSystemThoughtStore) GetUserState(userID string) UserState {
//    for _, u := range f.GetAllUserStates() {
//        if u.UserID == userID {
//            return u
//        }
//    }
//    return UserState{}
//}

func (f *FileSystemThoughtStore) GetThoughts(userID, subject string) []string {
    db, err := f.load()
    if err != nil { return nil }
    for _, u := range db {
        if u.UserID != userID { continue }
        for _, s := range u.Subjects {
            if s.Name == subject {
                out := make([]string, len(s.Thoughts))
                copy(out, s.Thoughts)
                return out
            }
        }
        return nil // user found, subject missing
    }
    return nil // user missing
}

// TODO: fix. base impl, still unsafe since not truncating
func (f *FileSystemThoughtStore) CaptureThought(userID, subject, thought string) {
    // 1) load current state
    db, err := f.load()
    if err != nil {
        return // keep simple for now; you’ll add error returns later
    }

    // 2) find or create the user row
    ui := -1
    for i := range db {
        if db[i].UserID == userID {
            ui = i
            break
        }
    }
    if ui == -1 {
        db = append(db, userRow{UserID: userID})
        ui = len(db) - 1
    }

    // 3) find or create the subject row for that user
    si := -1
    for i := range db[ui].Subjects {
        if db[ui].Subjects[i].Name == subject {
            si = i
            break
        }
    }
    if si == -1 {
        db[ui].Subjects = append(db[ui].Subjects, subjectRow{
            Name:     subject,
            Thoughts: []string{thought},
        })
    } else {
        db[ui].Subjects[si].Thoughts = append(db[ui].Subjects[si].Thoughts, thought)
    }

    // 4) write back (seek→encode). No truncation yet; fine for growing JSON.
    _, _ = f.database.Seek(0, 0)
    _ = json.NewEncoder(f.database).Encode(db)
}




