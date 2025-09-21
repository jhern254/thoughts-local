package data

import "sync"

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
func (i *InMemoryThoughtStore) CaptureThought(userID, subject, thought string) {
    i.lock.Lock()
    defer i.lock.Unlock()
    i.thoughts[subject] = append(i.thoughts[subject], thought)            
}

