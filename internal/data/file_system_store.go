// file_system_store.go 
package data

import (
    "fmt"
    "io"
    "sync"
    "os"
//    "net/http"
//    "errors"
//    "strings"
    "encoding/json"
    "context"
)

type FileSystemThoughtStore struct {
    database   *json.Encoder
    cache      dbFile      // cache

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

// populate cache from file
func loadFrom(file *os.File) (cache dbFile, err error) {
    if _, err := file.Seek(0, 0); err != nil {
        return nil, err
	}

    info, err := file.Stat()

    if err != nil {
        return nil, fmt.Errorf("problem getting file info from file %s %v", file.Name(), err)
    }

    // create empty json file if file is empty
    if info.Size() == 0 {
        file.Write([]byte("[]"))
        file.Seek(0, 0)
    }

	var c dbFile
	err = json.NewDecoder(file).Decode(&c)
    switch err {
	case nil:
		// Decoded OK. If the JSON was "null" somehow, normalize to [].
		if c == nil {
			c = dbFile{}
		}
		return c, nil
	case io.EOF:
		// Empty file → treat as empty dataset.
		return dbFile{}, nil
	default:
		// Any other error is a real decode problem.
		return nil, err
	}
}

func NewFileSystemThoughtStore(file *os.File) (*FileSystemThoughtStore, error) {
    // Validate we can read/parse whatever is there (including empty file).
    rows, err := loadFrom(file)
    if err != nil {
        return nil, fmt.Errorf("problem parsing thought store file: %w", err)
    }

    return &FileSystemThoughtStore{
        database: json.NewEncoder(&tape{file}),
        cache:     rows,
    }, nil
}

func (f *FileSystemThoughtStore) GetThoughts(userID, subject string) []string {
    f.lock.RLock()
    defer f.lock.RUnlock()

    for _, u := range f.cache {
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
    for i := range f.cache {
        if f.cache[i].UserID == userID {
            ui = i
            break
        }
    }
    if ui == -1 {
        f.cache = append(f.cache, userRow{UserID: userID})
        ui = len(f.cache) - 1
    }

    // Update existing subject or add a new one
    for si := range f.cache[ui].Subjects {
        if f.cache[ui].Subjects[si].Name == subject {
            f.cache[ui].Subjects[si].Thoughts =
                append(f.cache[ui].Subjects[si].Thoughts, thought)
            _ = f.database.Encode(f.cache) // persist
            return
        }
    }

    f.cache[ui].Subjects = append(f.cache[ui].Subjects, subjectRow{
        Name:     subject,
        Thoughts: []string{thought},
    })
    _ = f.database.Encode(f.cache) // persist
}

// Dummy implementations for SubjectStore
func (f *FileSystemThoughtStore) GetSubject(ctx context.Context, userID, subject string) (*Subject, error) {
    return nil, ErrRecordNotFound // minimal: always “not found”
}

func (f *FileSystemThoughtStore) CaptureSubject(ctx context.Context, userID, subject string) (int64, error) {
    return 0, nil // do nothing
}



