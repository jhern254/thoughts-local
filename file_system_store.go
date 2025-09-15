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
// like db struct
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
	err := json.NewDecoder(f.database).Decode(&db)
    switch err {
	case nil:
		// Decoded OK. If the JSON was "null" somehow, normalize to [].
		if db == nil {
			db = dbFile{}
		}
		return db, nil
	case io.EOF:
		// Empty file → treat as empty dataset.
		return dbFile{}, nil
	default:
		// Any other error is a real decode problem.
		return nil, err
	}
}

func (f *FileSystemThoughtStore) GetThoughts(userID, subject string) []string {
    db, err := f.load()
    if err != nil { 
        return nil 
    }
    // Scan users → subjects and return the first match.
    for _, u := range db {
        for _, s := range u.Subjects {
			if s.Name == subject {
				out := make([]string, len(s.Thoughts))
				copy(out, s.Thoughts)
				return out
			}
		}
	}
    // Not found → nil signals 404 to the handler in your tests.
	return nil
}

// TODO: fix. base impl, still unsafe since not truncating
func (f *FileSystemThoughtStore) CaptureThought(userID, subject, thought string) {
    db, err := f.load()
    if err != nil {
    	return 
    }

    // Ensure at least one user so we have a place to attach a subject.
    if len(db) == 0 {
    	db = append(db, userRow{UserID: "1"})
    }

    // Try to find the subject on any user (first match wins).
    for ui := range db {
    	for si := range db[ui].Subjects {
    		if db[ui].Subjects[si].Name == subject {
    			db[ui].Subjects[si].Thoughts =
    				append(db[ui].Subjects[si].Thoughts, thought)

    			// Rewind and write the updated JSON back.
    			// (No truncate yet; safe for growing JSON.)
    			_, _ = f.database.Seek(0, 0)
    			_ = json.NewEncoder(f.database).Encode(db)
    			return
    		}
    	}
    }

    // Subject not found → create it on the first user.
    db[0].Subjects = append(db[0].Subjects, subjectRow{
    	Name:     subject,
    	Thoughts: []string{thought},
    })

    // Rewind and write the updated JSON.
    _, _ = f.database.Seek(0, 0)
    _ = json.NewEncoder(f.database).Encode(db)
}




