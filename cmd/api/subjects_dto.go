package main

import (
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
)

type subjectCreateRequest struct {
	SubjectName string `json:"subject_name"`
}

type subjectResponse struct {
	SubjectID   int64  `json:"subject_id"`
	UserID      string `json:"user_id"`
	SubjectName string `json:"subject_name"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func toSubjectResponse(subject *data.Subject) subjectResponse {
	return subjectResponse{
		SubjectID:   subject.SubjectID,
		UserID:      subject.UserID,
		SubjectName: subject.SubjectName,
		CreatedAt:   subject.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:   subject.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
