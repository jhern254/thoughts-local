package subjectdiscovery

import (
	"context"
	"errors"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
)

type subjectReaderStub struct {
	list    func(context.Context, string) ([]data.Subject, error)
	aliases func(context.Context, string) ([]data.SubjectAlias, error)
}

func (s subjectReaderStub) List(ctx context.Context, user string) ([]data.Subject, error) {
	return s.list(ctx, user)
}
func (s subjectReaderStub) ListAliasesForUser(ctx context.Context, user string) ([]data.SubjectAlias, error) {
	return s.aliases(ctx, user)
}

type usageReaderFunc func(context.Context, string) ([]data.SubjectThoughtCountView, error)

func (f usageReaderFunc) ThoughtCountsBySubject(ctx context.Context, user string) ([]data.SubjectThoughtCountView, error) {
	return f(ctx, user)
}

type rankFunc func(context.Context, string, []Candidate) ([]Match, error)

func (f rankFunc) Rank(ctx context.Context, cue string, candidates []Candidate) ([]Match, error) {
	return f(ctx, cue, candidates)
}

func TestService_Load(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		aliasFailure, usageFailure bool
	}{
		{name: "loads one candidate per canonical Subject"},
		{name: "alias failure retains canonical matching", aliasFailure: true},
		{name: "usage failure retains names and aliases", usageFailure: true},
		{name: "both optional failures leave editor usable", aliasFailure: true, usageFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := []string{}
			problem := errors.New("private source failure")
			reader := subjectReaderStub{
				list: func(ctx context.Context, user string) ([]data.Subject, error) {
					calls = append(calls, "subjects")
					if ctx != t.Context() || user != "u" {
						t.Fatal("request ownership not propagated")
					}
					return []data.Subject{{SubjectID: 7, UserID: "u", SubjectName: "Go Programming"}}, nil
				},
				aliases: func(context.Context, string) ([]data.SubjectAlias, error) {
					calls = append(calls, "aliases")
					if tc.aliasFailure {
						return nil, problem
					}
					return []data.SubjectAlias{{UserID: "u", SubjectID: 7, AliasName: "Programming in Go"}, {UserID: "u", SubjectID: 7, AliasName: "Golang"}, {UserID: "foreign", SubjectID: 7, AliasName: "unowned"}}, nil
				},
			}
			usage := usageReaderFunc(func(context.Context, string) ([]data.SubjectThoughtCountView, error) {
				calls = append(calls, "usage")
				if tc.usageFailure {
					return nil, problem
				}
				return []data.SubjectThoughtCountView{{SubjectID: 7, Count: 12}}, nil
			})
			service := NewService(reader, usage, NameRecommender{})
			result, err := service.Load(t.Context(), "u")
			if err != nil || len(result.Snapshot.Candidates) != 1 || len(calls) != 3 {
				t.Fatalf("got %+v,%v calls=%v", result, err, calls)
			}
			if (result.AliasErr != nil) != tc.aliasFailure || (result.UsageErr != nil) != tc.usageFailure {
				t.Fatal("optional failure not reported")
			}
			candidate := result.Snapshot.Candidates[0]
			if candidate.UsageKnown == tc.usageFailure || (!tc.aliasFailure && len(candidate.Aliases) != 2) {
				t.Fatalf("candidate=%+v", candidate)
			}
			resolved, err := service.Resolve(t.Context(), "Go Programming", result.Snapshot)
			if err != nil || resolved.Subject == nil || resolved.Subject.SubjectID != 7 {
				t.Fatal("canonical matching unavailable")
			}
		})
	}
	t.Run("canonical failure stops further reads", func(t *testing.T) {
		problem := errors.New("read failure")
		service := NewService(subjectReaderStub{list: func(context.Context, string) ([]data.Subject, error) { return nil, problem }}, nil, NameRecommender{})
		if _, err := service.Load(t.Context(), "u"); !errors.Is(err, problem) {
			t.Fatalf("got %v want read failure", err)
		}
	})
	t.Run("cancellation stops obsolete reads", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		service := NewService(subjectReaderStub{list: func(context.Context, string) ([]data.Subject, error) { cancel(); return nil, nil }}, nil, NameRecommender{})
		if _, err := service.Load(ctx, "u"); !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v want cancellation", err)
		}
	})
}

func TestSnapshot_ConfirmsCommand(t *testing.T) {
	snapshot := NewSnapshot([]Candidate{{
		Subject:      data.Subject{SubjectID: 1, SubjectName: "I Ching"},
		Aliases:      []string{"The Art of Learning", "I Want to Learn Go", "My Learning Journal", "Remember Tomorrow", "Yesterday I", "My first", "It was"},
		ThoughtCount: 1_000_000,
		UsageKnown:   true,
	}})
	for _, tc := range []struct {
		text string
		want bool
	}{
		{"I Ching thought, review notes.", true},
		{"The Art of Learning thought, review notes.", true},
		{"I Want to Learn Go thought, review notes.", true},
		{"My Learning Journal thought, review notes.", true},
		{"Remember Tomorrow thought, review notes.", true},
		{"i ching thought, review notes.", true},
		{"Random thought, perhaps I should learn Go.", false},
		{"My unknown journal thought, review notes.", false},
		{"Yesterday I thought about Go.", false},
		{"My first thought was to practice.", false},
		{"It was thought to be finished.", false},
		{"Machine Learning Research thought, study embeddings.", true},
	} {
		t.Run(tc.text, func(t *testing.T) {
			prefix, _ := ParseSpokenPrefix(tc.text)
			if got := snapshot.ConfirmsCommand(prefix); got != tc.want {
				t.Fatalf("confirmed=%v, want %v (prefix=%+v)", got, tc.want, prefix)
			}
		})
	}
	t.Run("absent candidate data cannot confirm an uncertain name", func(t *testing.T) {
		prefix, _ := ParseSpokenPrefix("I Ching thought, review notes.")
		if (Snapshot{}).ConfirmsCommand(prefix) {
			t.Fatal("empty snapshot confirmed a possible command")
		}
	})
}
func TestService_VoiceNameIdentity(t *testing.T) {
	t.Run("unique folded identity bypasses heuristic ranking", func(t *testing.T) {
		snapshot := NewSnapshot([]Candidate{{Subject: data.Subject{SubjectID: 1, SubjectName: "coding"}, Aliases: []string{"coDing"}}})
		service := &Service{recommender: rankFunc(func(context.Context, string, []Candidate) ([]Match, error) {
			t.Fatal("exact match invoked ranking")
			return nil, nil
		})}
		for _, cue := range []string{"coding", "coDing", "CODING"} {
			result, err := service.Resolve(t.Context(), cue, snapshot)
			if err != nil || result.Subject == nil || result.Subject.SubjectID != 1 {
				t.Fatalf("cue %q got %+v,%v", cue, result, err)
			}
		}
	})
	t.Run("canceled ranking returns without selection", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		service := &Service{recommender: NameRecommender{}}
		if _, err := service.Resolve(ctx, "coding", Snapshot{}); !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v want cancellation", err)
		}
	})
}

func TestService_VoiceNames(t *testing.T) {
	for _, tc := range []struct {
		cue        string
		candidates []Candidate
		wantID     int64
	}{
		{"fitness", []Candidate{{Subject: data.Subject{SubjectID: 1, SubjectName: "Fitness"}}}, 1},
		{"FITNESS", []Candidate{{Subject: data.Subject{SubjectID: 1, SubjectName: "Fitness"}}}, 1},
		{"work", []Candidate{{Subject: data.Subject{SubjectID: 1, SubjectName: "Work"}}}, 1},
		{"golang", []Candidate{{Subject: data.Subject{SubjectID: 7, SubjectName: "Go Programming"}, Aliases: []string{"Golang", "GOLANG"}}}, 7},
		{"études", []Candidate{{Subject: data.Subject{SubjectID: 1, SubjectName: "ÉTUDES"}}}, 1},
		{"coding", []Candidate{{Subject: data.Subject{SubjectID: 1, SubjectName: "coding"}, ThoughtCount: 1000000, UsageKnown: true}, {Subject: data.Subject{SubjectID: 2, SubjectName: "coDing"}}}, 0},
		{"CODING", []Candidate{{Subject: data.Subject{SubjectID: 1, SubjectName: "coding"}}, {Subject: data.Subject{SubjectID: 2, SubjectName: "Other"}, Aliases: []string{"coDing"}}}, 0},
	} {
		t.Run(tc.cue, func(t *testing.T) {
			service := &Service{recommender: rankFunc(func(context.Context, string, []Candidate) ([]Match, error) {
				t.Fatal("voice identity matching invoked fuzzy ranking")
				return nil, nil
			})}
			got, err := service.Resolve(t.Context(), tc.cue, NewSnapshot(tc.candidates))
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantID == 0 {
				if got.Subject != nil || len(got.Matches) != 0 {
					t.Fatalf("got %+v, want no selection or candidates", got)
				}
			} else if got.Subject == nil || got.Subject.SubjectID != tc.wantID {
				t.Fatalf("got %+v, want Subject %d", got, tc.wantID)
			}
		})
	}
}
