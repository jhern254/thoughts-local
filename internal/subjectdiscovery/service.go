// Package subjectdiscovery supplies read-only, provisional Subject suggestions.
// Voice spelling comparisons never change persisted Subject identity.
package subjectdiscovery

import (
	"context"
	"strings"

	"github.com/jhern254/go-thoughts/internal/data"
)

type SubjectReader interface {
	List(context.Context, string) ([]data.Subject, error)
	ListAliasesForUser(context.Context, string) ([]data.SubjectAlias, error)
}
type UsageReader interface {
	ThoughtCountsBySubject(context.Context, string) ([]data.SubjectThoughtCountView, error)
}

type Candidate struct {
	Subject      data.Subject
	Aliases      []string
	ThoughtCount int64
	UsageKnown   bool
	names        []nameFeatures
}

type Snapshot struct{ Candidates []Candidate }

// ConfirmsCommand adds unique case-insensitive voice-name evidence to a possible
// command. Narrative vetoes survive even a unique match; ranking/usage are not
// evidence of intent. An empty snapshot leaves uncertain dictation untouched.
func (s Snapshot) ConfirmsCommand(prefix SpokenPrefix) bool {
	match := s.MatchVoiceName(prefix.Cue)
	if match.Outcome == NameCollision {
		return false
	}
	switch prefix.Intent {
	case ConfirmedCommand:
		return true
	case PossibleCommand:
		return match.Subject != nil
	default:
		return false
	}
}

// MatchVoiceName ignores transcription capitalization, counting canonical IDs
// rather than spellings. A collision is terminal: neither exact casing, usage,
// nor fuzzy ranking may choose between distinct persisted identities.
func (s Snapshot) MatchVoiceName(cue string) Result {
	var matched *data.Subject
	for _, candidate := range s.Candidates {
		matches := strings.EqualFold(candidate.Subject.SubjectName, cue)
		for _, alias := range candidate.Aliases {
			matches = matches || strings.EqualFold(alias, cue)
		}
		if !matches {
			continue
		}
		if matched != nil && matched.SubjectID != candidate.Subject.SubjectID {
			return Result{Outcome: NameCollision}
		}
		matched = &candidate.Subject
	}
	if matched != nil {
		return Result{Outcome: Existing, Subject: matched}
	}
	return Result{Outcome: Unknown}
}

// NewSnapshot prepares immutable matching features once per voice draft.
func NewSnapshot(candidates []Candidate) Snapshot {
	result := Snapshot{Candidates: make([]Candidate, 0, len(candidates))}
	seen := make(map[int64]bool, len(candidates))
	for _, candidate := range candidates {
		if seen[candidate.Subject.SubjectID] {
			continue
		}
		seen[candidate.Subject.SubjectID] = true
		candidate.Aliases = append([]string(nil), candidate.Aliases...)
		candidate.names = []nameFeatures{extractNameFeatures(candidate.Subject.SubjectName)}
		for _, name := range candidate.Aliases {
			candidate.names = append(candidate.names, extractNameFeatures(name))
		}
		result.Candidates = append(result.Candidates, candidate)
	}
	return result
}

type LoadResult struct {
	Snapshot Snapshot
	AliasErr error
	UsageErr error
}

type Service struct {
	subjects    SubjectReader
	usage       UsageReader
	recommender Recommender
}

func NewService(subjects SubjectReader, usage UsageReader, recommender Recommender) *Service {
	return &Service{subjects: subjects, usage: usage, recommender: recommender}
}

// Load uses the existing APIs once each. Optional failures retain canonical
// matching; callers classify errors at their presentation boundary.
func (s *Service) Load(ctx context.Context, userID string) (LoadResult, error) {
	subjects, err := s.subjects.List(ctx, userID)
	if err != nil {
		return LoadResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return LoadResult{}, err
	}
	aliases, aliasErr := s.subjects.ListAliasesForUser(ctx, userID)
	if err := ctx.Err(); err != nil {
		return LoadResult{}, err
	}
	counts, usageErr := s.usage.ThoughtCountsBySubject(ctx, userID)
	if err := ctx.Err(); err != nil {
		return LoadResult{}, err
	}
	candidates := make([]Candidate, len(subjects))
	positions := make(map[int64]int, len(subjects))
	for index, item := range subjects {
		candidates[index].Subject = item
		positions[item.SubjectID] = index
	}
	if aliasErr == nil {
		for _, alias := range aliases {
			if index, ok := positions[alias.SubjectID]; ok && alias.UserID == userID {
				candidates[index].Aliases = append(candidates[index].Aliases, alias.AliasName)
			}
		}
	}
	if usageErr == nil {
		for _, count := range counts {
			if index, ok := positions[count.SubjectID]; ok {
				candidates[index].ThoughtCount = count.Count
				candidates[index].UsageKnown = true
			}
		}
	}
	return LoadResult{Snapshot: NewSnapshot(candidates), AliasErr: aliasErr, UsageErr: usageErr}, nil
}

type Outcome uint8

const (
	Unknown Outcome = iota
	Existing
	Ambiguous
	NameCollision
)

type Result struct {
	Outcome Outcome
	Subject *data.Subject
	Matches []data.Subject
}

// Resolve separates voice-name matching and confidence decisions from the scoring
// algorithm. It performs no I/O and is safe inside a synchronous draft mutation.
func (s *Service) Resolve(ctx context.Context, cue string, snapshot Snapshot) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if match := snapshot.MatchVoiceName(cue); match.Outcome != Unknown {
		return match, nil
	}
	matches, err := s.recommender.Rank(ctx, cue, snapshot.Candidates)
	if err != nil {
		return Result{}, err
	}
	result := Result{Outcome: Unknown}
	var relevant []Match
	for _, match := range matches {
		if match.Similarity >= minimumSimilarity {
			relevant = append(relevant, match)
			result.Matches = append(result.Matches, match.Candidate.Subject)
		}
	}
	if len(relevant) == 0 {
		return result, nil
	}
	best := relevant[0]
	if len(relevant) == 1 || best.Similarity-relevant[1].Similarity >= clearNameMargin {
		result.Outcome = Existing
		result.Subject = &best.Candidate.Subject
		return result, nil
	}
	// Usage only breaks close name ties. A clear name winner bypasses usage,
	// and unrelated candidates never enter this shortlist, however popular.
	shortlist := relevant[:0]
	for _, match := range relevant {
		if best.Similarity-match.Similarity < clearNameMargin {
			shortlist = append(shortlist, match)
		}
	}
	winner := -1
	for index, match := range shortlist {
		if !match.Candidate.UsageKnown || match.Candidate.ThoughtCount < minimumUsageLead {
			continue
		}
		clear := true
		for otherIndex, other := range shortlist {
			if otherIndex == index {
				continue
			}
			count, otherCount := match.Candidate.ThoughtCount, other.Candidate.ThoughtCount
			if !other.Candidate.UsageKnown || count-otherCount < minimumUsageLead || count/2 < otherCount {
				clear = false
				break
			}
		}
		if clear {
			winner = index
			break
		}
	}
	if winner >= 0 {
		result.Outcome = Existing
		result.Subject = &shortlist[winner].Candidate.Subject
		return result, nil
	}
	result.Outcome = Ambiguous
	return result, nil
}
