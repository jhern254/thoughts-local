package data

import (
	"context"
	"sync"
)

type InMemoryStore struct {
	subjects                     map[int64]Subject
	thoughts                     map[int64]Thought
	nextSubjectID, nextThoughtID int64
	mu                           sync.RWMutex
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{subjects: map[int64]Subject{}, thoughts: map[int64]Thought{}}
}
func (s *InMemoryStore) CreateSubject(ctx context.Context, subject *Subject) (*Subject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.subjects {
		if v.UserID == subject.UserID && v.SubjectName == subject.SubjectName {
			return nil, ErrDuplicateRecord
		}
	}
	s.nextSubjectID++
	out := *subject
	out.SubjectID = s.nextSubjectID
	s.subjects[out.SubjectID] = out
	return &out, nil
}
func (s *InMemoryStore) GetSubject(ctx context.Context, user string, id int64) (*Subject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.subjects[id]
	if !ok || v.UserID != user {
		return nil, ErrRecordNotFound
	}
	return &v, nil
}
func (s *InMemoryStore) CreateThought(ctx context.Context, thought *Thought) (*Thought, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if thought.SubjectID != nil {
		v, ok := s.subjects[*thought.SubjectID]
		if !ok || v.UserID != thought.UserID {
			return nil, ErrRecordNotFound
		}
	}
	s.nextThoughtID++
	out := *thought
	out.ThoughtID = s.nextThoughtID
	s.thoughts[out.ThoughtID] = out
	return &out, nil
}
func (s *InMemoryStore) GetThought(ctx context.Context, user string, id int64) (*Thought, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.thoughts[id]
	if !ok || v.UserID != user {
		return nil, ErrRecordNotFound
	}
	return &v, nil
}
