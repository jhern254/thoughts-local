package data

// composite store
type Store interface {
	ThoughtStore
	SubjectStore
	// TagStore
	// EventStore
}
