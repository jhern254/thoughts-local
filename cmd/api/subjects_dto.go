package main

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
