package data

import (
    "sync"
    "context"
)

func NewInMemoryThoughtStore() *InMemoryThoughtStore {
    return &InMemoryThoughtStore{
        map[string][]string{},
        sync.RWMutex{}, 
    } 
}

type InMemoryThoughtStore struct {
    thoughts map[string][]string
    // mutex to synch read/write access to map
    lock     sync.RWMutex
}

// TODO: add userID impl
func (i *InMemoryThoughtStore) GetThoughts(userID, subject string) []string {
    i.lock.Lock()
    defer i.lock.Unlock()
    return i.thoughts[subject]
}

// TODO: add userID impl
func (i *InMemoryThoughtStore) CaptureThought(ctx context.Context, userID, subject, thought string) (int64, error) { i.lock.Lock()
    i.lock.Lock()
    defer i.lock.Unlock()
    // Init map if needed
    if i.thoughts == nil {
        i.thoughts = make(map[string][]string)
    }

    // Append thought
    i.thoughts[subject] = append(i.thoughts[subject], thought)

    // "ID" = 1-based index in this subject's slice (matches the filesystem stub pattern)
    newID := int64(len(i.thoughts[subject]))
    return newID, nil
}

// Dummy implementations for SubjectStore
func (i *InMemoryThoughtStore) GetSubject(ctx context.Context, userID, subject string) (*Subject, error) {
    return nil, ErrRecordNotFound // minimal: always “not found”
}

func (i *InMemoryThoughtStore) CaptureSubject(ctx context.Context, userID string, subject *Subject) (int64, error) {
    return 0, nil // do nothing
}

