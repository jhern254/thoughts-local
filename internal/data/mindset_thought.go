package data

import "time"

// MindsetThought carries complete text for the reading panel, unlike a bounded
// ThoughtSummaryView. Historical membership still displays current content.
type MindsetThought struct {
	ThoughtID   int64
	Text        string
	SubjectName *string
	ObservedAt  time.Time
	CreatedAt   time.Time
	IsMindset   bool
}
