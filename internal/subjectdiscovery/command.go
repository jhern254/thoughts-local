package subjectdiscovery

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// These are spoken-command limits, not Subject persistence limits. Long names
// remain valid Subjects and can still be selected manually.
const (
	maximumCueCharacters = 255
	maximumCueWords      = 8
)

// SpokenPrefix offsets refer to bytes in the original transcript, so removing
// a prefix never slices a Unicode character or reconstructs the dictated body.
type SpokenPrefix struct {
	Cue       string
	BodyStart int
}

// ParseSpokenPrefix recognizes only a short leading name before the first
// standalone "thought". This deliberately conservative English command grammar
// is independent of whether the name exists; Unicode names are retained verbatim.
func ParseSpokenPrefix(text string) (SpokenPrefix, bool) {
	wordStart := -1
	characters := 0
	for offset, character := range text {
		characters++
		if characters > maximumCueCharacters+len(" thought ") {
			return SpokenPrefix{}, false
		}
		if character == utf8.RuneError {
			return SpokenPrefix{}, false
		}
		wordCharacter := unicode.IsLetter(character) || unicode.IsNumber(character) || unicode.IsMark(character) || character == '\'' || character == '’'
		if wordCharacter {
			if wordStart < 0 {
				wordStart = offset
			}
			continue
		}
		if wordStart >= 0 && strings.EqualFold(text[wordStart:offset], "thought") {
			return spokenPrefix(text, wordStart, offset)
		}
		wordStart = -1
	}
	if wordStart >= 0 && strings.EqualFold(text[wordStart:], "thought") {
		return spokenPrefix(text, wordStart, len(text))
	}
	return SpokenPrefix{}, false
}

func spokenPrefix(text string, start, end int) (SpokenPrefix, bool) {
	cue := strings.TrimSpace(text[:start])
	words := strings.Fields(cue)
	if len(words) == 0 || len(words) > maximumCueWords || utf8.RuneCountInString(cue) > maximumCueCharacters || strings.ContainsAny(cue, "\n\r;!?") {
		return SpokenPrefix{}, false
	}
	// Pronoun/article-led fragments are more likely narrative than a Subject cue.
	// Declining these phrases is safer than removing ordinary dictated sentences.
	switch strings.ToLower(strings.Trim(words[0], "\"'“”")) {
	case "i", "i'm", "i’ve", "i've", "i’d", "i'd", "i’ll", "i'll", "we", "we're", "we’ve", "we've", "you", "you're", "he", "she", "it", "they", "this", "that", "these", "those", "there", "here", "a", "an", "the", "another", "some", "my", "our", "your", "remember":
		return SpokenPrefix{}, false
	}
	for end < len(text) {
		character, size := utf8.DecodeRuneInString(text[end:])
		if !unicode.IsSpace(character) && !strings.ContainsRune(",.:;!?-–—，。：；！？", character) {
			break
		}
		end += size
	}
	return SpokenPrefix{Cue: cue, BodyStart: end}, true
}
