package subjectdiscovery

import (
	"context"
	"sort"
	"strings"
	"unicode"
)

// Heuristic tuning, not calibrated probabilities. Usage cannot compensate for
// a name below minimumSimilarity or overturn a clearNameMargin winner.
const (
	minimumSimilarity = 0.75
	clearNameMargin   = 0.10
	minimumUsageLead  = int64(3)
)

type Match struct {
	Candidate  Candidate
	Similarity float64
}

// Recommender ranks supplied candidates using CPU-only, cancellation-aware work.
// Implementations must not perform I/O or wait: live draft updates own the call.
type Recommender interface {
	Rank(context.Context, string, []Candidate) ([]Match, error)
}
type NameRecommender struct{}

type nameFeatures struct {
	words    map[string]struct{}
	trigrams map[string]struct{}
}

func extractNameFeatures(name string) nameFeatures {
	tokens := strings.FieldsFunc(strings.ToLower(name), func(character rune) bool {
		return !unicode.IsLetter(character) && !unicode.IsNumber(character) && !unicode.IsMark(character)
	})
	words := make(map[string]struct{}, len(tokens))
	significant := make([]string, 0, len(tokens))
	for _, token := range tokens {
		switch token {
		case "a", "an", "the", "in", "of", "for", "and", "to", "on":
			continue
		}
		words[token] = struct{}{}
		significant = append(significant, token)
	}
	phrase := []rune(strings.Join(significant, " "))
	trigrams := make(map[string]struct{}, len(phrase))
	if len(phrase) < 3 && len(phrase) > 0 {
		trigrams[string(phrase)] = struct{}{}
	}
	for index := 0; index+3 <= len(phrase); index++ {
		trigrams[string(phrase[index:index+3])] = struct{}{}
	}
	return nameFeatures{words: words, trigrams: trigrams}
}
func diceOverlap(first, second map[string]struct{}) float64 {
	if len(first)+len(second) == 0 {
		return 0
	}
	common := 0
	for token := range first {
		if _, ok := second[token]; ok {
			common++
		}
	}
	return float64(2*common) / float64(len(first)+len(second))
}
func (NameRecommender) Rank(ctx context.Context, cue string, candidates []Candidate) ([]Match, error) {
	query := extractNameFeatures(cue)
	matches := make([]Match, 0, len(candidates))
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		score := 0.0
		for _, name := range candidate.names {
			score = max(score, diceOverlap(query.words, name.words), diceOverlap(query.trigrams, name.trigrams))
		}
		matches = append(matches, Match{Candidate: candidate, Similarity: score})
	}
	sort.Slice(matches, func(first, second int) bool {
		if matches[first].Similarity != matches[second].Similarity {
			return matches[first].Similarity > matches[second].Similarity
		}
		return matches[first].Candidate.Subject.SubjectID < matches[second].Candidate.Subject.SubjectID
	})
	return matches, nil
}
