package data

// CompositeStore combines the temporary thought store with the SQLite subject store.
type CompositeStore struct {
	ThoughtStore
	SubjectStore
}

func NewCompositeStore(thoughts ThoughtStore, subjects SubjectStore) Store {
	return CompositeStore{
		ThoughtStore: thoughts,
		SubjectStore: subjects,
	}
}
