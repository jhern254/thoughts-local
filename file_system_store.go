// file_system_store.go 
package main

import (
    "fmt"
    "io"
    "sync"
//    "os"
//    "net/http"
//    "errors"
//    "strings"
    "encoding/json"
)

type FileSystemThoughtStore struct {
    writer  io.Writer
    db      dbFile      // cache
    lock    sync.RWMutex
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

func loadFrom(rws io.ReadWriteSeeker) (dbFile, error) {
    if _, err := rws.Seek(0, 0); err != nil {
        return nil, err
	}

	var db dbFile
	err := json.NewDecoder(rws).Decode(&db)
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

func NewFileSystemThoughtStore(rws io.ReadWriteSeeker) (*FileSystemThoughtStore, error) {
    // Validate we can read/parse whatever is there (including empty file).
    rows, err := loadFrom(rws)
    if err != nil {
        return nil, fmt.Errorf("problem parsing thought store file: %w", err)
    }

    return &FileSystemThoughtStore{
        writer: rws,
        db:     rows,
    }, nil
}

func (f *FileSystemThoughtStore) GetThoughts(userID, subject string) []string {
    f.lock.RLock()
    defer f.lock.RUnlock()

    for _, u := range f.db {
        if u.UserID != userID { continue }
        for _, s := range u.Subjects {
            if s.Name == subject {
                out := make([]string, len(s.Thoughts))
                copy(out, s.Thoughts) // protect internal slice
                return out
            }
        }
        return nil // user found, subject missing
    }
    return nil // user missing
}

// TODO: fix. base impl, still unsafe since not truncating
func (f *FileSystemThoughtStore) CaptureThought(userID, subject, thought string) {
    f.lock.Lock()
    defer f.lock.Unlock()

    // Ensure the user row exists (prefer the provided userID)
    // writes to user
    ui := -1
    for i := range f.db {
        if f.db[i].UserID == userID {
            ui = i
            break
        }
    }
    if ui == -1 {
        f.db = append(f.db, userRow{UserID: userID})
        ui = len(f.db) - 1
    }

    // Update existing subject or add a new one
    for si := range f.db[ui].Subjects {
        if f.db[ui].Subjects[si].Name == subject {
            f.db[ui].Subjects[si].Thoughts =
                append(f.db[ui].Subjects[si].Thoughts, thought)
            _ = json.NewEncoder(f.writer).Encode(f.db) // persist
            return
        }
    }

    f.db[ui].Subjects = append(f.db[ui].Subjects, subjectRow{
        Name:     subject,
        Thoughts: []string{thought},
    })
    _ = json.NewEncoder(f.writer).Encode(f.db) // persist
}




