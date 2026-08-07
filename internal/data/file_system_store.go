package data

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

type FileSystemStore struct {
	file *os.File
	data jsonData
	mu   sync.RWMutex
}

type jsonData struct {
	NextSubjectID int64     `json:"next_subject_id"`
	NextThoughtID int64     `json:"next_thought_id"`
	Subjects      []Subject `json:"subjects"`
	Thoughts      []Thought `json:"thoughts"`
}

func NewFileSystemStore(file *os.File) (*FileSystemStore, error) {
	if _, err := file.Seek(0, 0); err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	data := jsonData{}
	if info.Size() > 0 {
		if err := json.NewDecoder(file).Decode(&data); err != nil {
			return nil, fmt.Errorf("read table-shaped JSON store: %w", err)
		}
	}
	return &FileSystemStore{file: file, data: data}, nil
}

func (s *FileSystemStore) persist() error {
	if err := s.file.Truncate(0); err != nil {
		return err
	}
	if _, err := s.file.Seek(0, 0); err != nil {
		return err
	}
	if err := json.NewEncoder(s.file).Encode(s.data); err != nil {
		return err
	}
	return s.file.Sync()
}

func (s *FileSystemStore) CreateSubject(ctx context.Context, subject *Subject) (*Subject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.data.Subjects {
		if existing.UserID == subject.UserID && existing.SubjectName == subject.SubjectName {
			return nil, ErrDuplicateRecord
		}
	}
	s.data.NextSubjectID++
	stored := *subject
	stored.SubjectID = s.data.NextSubjectID
	s.data.Subjects = append(s.data.Subjects, stored)
	if err := s.persist(); err != nil {
		return nil, err
	}
	return &stored, nil
}

func (s *FileSystemStore) GetSubject(ctx context.Context, userID string, subjectID int64) (*Subject, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, subject := range s.data.Subjects {
		if subject.SubjectID == subjectID && subject.UserID == userID {
			copy := subject
			return &copy, nil
		}
	}
	return nil, ErrRecordNotFound
}

func (s *FileSystemStore) CreateThought(ctx context.Context, thought *Thought) (*Thought, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if thought.SubjectID != nil {
		found := false
		for _, subject := range s.data.Subjects {
			if subject.SubjectID == *thought.SubjectID && subject.UserID == thought.UserID {
				found = true
				break
			}
		}
		if !found {
			return nil, ErrRecordNotFound
		}
	}
	s.data.NextThoughtID++
	stored := *thought
	stored.ThoughtID = s.data.NextThoughtID
	s.data.Thoughts = append(s.data.Thoughts, stored)
	if err := s.persist(); err != nil {
		return nil, err
	}
	return &stored, nil
}

func (s *FileSystemStore) GetThought(ctx context.Context, userID string, thoughtID int64) (*Thought, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, thought := range s.data.Thoughts {
		if thought.ThoughtID == thoughtID && thought.UserID == userID {
			copy := thought
			return &copy, nil
		}
	}
	return nil, ErrRecordNotFound
}

var _ SubjectStore = (*FileSystemStore)(nil)
var _ ThoughtStore = (*FileSystemStore)(nil)
