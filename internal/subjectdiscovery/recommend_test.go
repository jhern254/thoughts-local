package subjectdiscovery

import (
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
)

func TestService_Resolve(t *testing.T) {
	goSubject := Candidate{Subject: data.Subject{SubjectID: 7, SubjectName: "Go Programming"}}
	goWithAliases := goSubject
	goWithAliases.Aliases = []string{"Programming in Go", "Golang", "Go development"}
	lowerCoding := Candidate{Subject: data.Subject{SubjectID: 1, SubjectName: "coding"}}
	mixedCoding := Candidate{Subject: data.Subject{SubjectID: 2, SubjectName: "coDing"}}
	upperCoding := Candidate{Subject: data.Subject{SubjectID: 3, SubjectName: "Coding"}}
	popularCoding, rareCoding := lowerCoding, upperCoding
	popularCoding.ThoughtCount, popularCoding.UsageKnown = 20, true
	rareCoding.ThoughtCount, rareCoding.UsageKnown = 5, true
	smallCoding, unusedCoding := lowerCoding, upperCoding
	smallCoding.ThoughtCount, smallCoding.UsageKnown = 2, true
	unusedCoding.UsageKnown = true
	unrelated := Candidate{
		Subject:      data.Subject{SubjectID: 8, SubjectName: "Gardening"},
		ThoughtCount: 1000000,
		UsageKnown:   true,
	}
	for _, tc := range []struct {
		name, cue  string
		candidates []Candidate
		kind       Outcome
		subjectID  int64
	}{
		{"unique canonical", "coding", []Candidate{lowerCoding}, Existing, 1},
		{"exact casing cannot override collision", "coding", []Candidate{lowerCoding, mixedCoding}, NameCollision, 0},
		{"exact alias", "Programming in Go", []Candidate{goWithAliases}, Existing, 7},
		{"word order", "Programming in Go", []Candidate{goSubject}, Existing, 7},
		{"similar phrase", "Go programming practice", []Candidate{goSubject}, Existing, 7},
		{"approximate alias", "GO DEVELOPMENT", []Candidate{goWithAliases}, Existing, 7},
		{"case collision", "CODING", []Candidate{lowerCoding, mixedCoding, upperCoding}, NameCollision, 0},
		{"usage cannot override collision", "CODING", []Candidate{popularCoding, rareCoding}, NameCollision, 0},
		{"usage breaks fuzzy tie", "coding-", []Candidate{popularCoding, rareCoding}, Existing, 1},
		{"small usage difference remains ambiguous", "coding-", []Candidate{smallCoding, unusedCoding}, Ambiguous, 0},
		{"missing counts cannot break fuzzy tie", "coding-", []Candidate{popularCoding, upperCoding}, Ambiguous, 0},
		{"popularity cannot override relevance", "Go programming practice", []Candidate{goSubject, unrelated}, Existing, 7},
		{"unknown", "Machine Learning Research", []Candidate{goSubject}, Unknown, 0},
		{"empty candidates", "Fitness", nil, Unknown, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := &Service{recommender: NameRecommender{}}
			got, err := service.Resolve(t.Context(), tc.cue, NewSnapshot(tc.candidates))
			if err != nil || got.Outcome != tc.kind {
				t.Fatalf("got %+v,%v, want outcome %v", got, err, tc.kind)
			}
			if tc.subjectID != 0 && (got.Subject == nil || got.Subject.SubjectID != tc.subjectID) {
				t.Fatalf("got %+v, want Subject %d", got.Subject, tc.subjectID)
			}
			if tc.subjectID == 0 && got.Subject != nil {
				t.Fatalf("got selected %+v, want no selection", got.Subject)
			}
		})
	}
	t.Run("several aliases produce only one canonical candidate", func(t *testing.T) {
		snapshot := NewSnapshot([]Candidate{{Subject: data.Subject{SubjectID: 1, SubjectName: "Go Programming"}, Aliases: []string{"Go development", "Programming in Go", "Golang"}}})
		matches, err := (NameRecommender{}).Rank(t.Context(), "Go programming practice", snapshot.Candidates)
		if err != nil || len(matches) != 1 {
			t.Fatalf("got %+v,%v, want one candidate", matches, err)
		}
	})
}
