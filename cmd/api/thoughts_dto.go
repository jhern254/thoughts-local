package main

import (
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

type thoughtCreateRequest struct {
	SubjectID  *int64  `json:"subject_id"`
	Thought    string  `json:"thought"`
	ObservedAt *string `json:"observed_at"`
}

type thoughtResponse struct {
	ThoughtID  int64  `json:"thought_id"`
	UserID     string `json:"user_id"`
	SubjectID  *int64 `json:"subject_id"`
	EventID    *int64 `json:"event_id"`
	Thought    string `json:"thought"`
	Version    int64  `json:"version"`
	ObservedAt string `json:"observed_at"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

func toThoughtResponse(thought *data.Thought) thoughtResponse {
	return thoughtResponse{
		ThoughtID:  thought.ThoughtID,
		UserID:     thought.UserID,
		SubjectID:  thought.SubjectID,
		EventID:    thought.EventID,
		Thought:    thought.Thought,
		Version:    thought.Version,
		ObservedAt: thought.ObservedAt.UTC().Format(time.RFC3339),
		CreatedAt:  thought.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:  thought.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
