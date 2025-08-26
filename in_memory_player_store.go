package main

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

func (i *InMemoryThoughtStore) GetThoughts(subject string) []string {
    i.lock.Lock()
    defer i.lock.Unlock()
    return i.thoughts[subject]
}

func (i *InMemoryThoughtStore) CaptureThought(subject, thought string) {
    i.lock.Lock()
    defer i.lock.Unlock()
    i.thoughts[subject] = append(i.thoughts[subject], thought)            
}

func (i *InMemoryThoughtStore) GetUserState(userID string) UserState {
    return UserState{}
}
