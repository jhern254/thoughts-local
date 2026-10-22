package subjectdiscovery

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// These are spoken-command limits, not Subject persistence limits. Long names
// remain valid Subjects and can still be selected manually.
const (
	maximumCueCharacters        = 255
	maximumCueWords             = 8
	maximumIntentWordCharacters = 32
)

type CommandIntent uint8

const (
	OrdinarySpeech CommandIntent = iota
	PossibleCommand
	ConfirmedCommand
)

// SpokenPrefix offsets refer to bytes in the original transcript, so removing
// a prefix never slices a Unicode character or reconstructs the dictated body.
type SpokenPrefix struct {
	Cue       string
	BodyStart int
	Intent    CommandIntent
}

// ParseSpokenPrefix recognizes only a short leading name before the first
// standalone "thought". This deliberately conservative English command grammar
// is independent of whether the name exists; Unicode names are retained verbatim.
// A true result includes PossibleCommand: callers must confirm intent before
// assigning a Subject or removing text, rather than treating a prefix as proof.
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
	for end < len(text) {
		character, size := utf8.DecodeRuneInString(text[end:])
		if !unicode.IsSpace(character) && !strings.ContainsRune(",.:;!?-–—，。：；！？", character) {
			break
		}
		end += size
	}
	intent := spokenCommandIntent(words, text[end:])
	if intent == OrdinarySpeech {
		return SpokenPrefix{}, false
	}
	return SpokenPrefix{Cue: cue, BodyStart: end, Intent: intent}, true
}

func spokenCommandIntent(words []string, body string) CommandIntent {
	for index, word := range words {
		words[index] = strings.ToLower(strings.TrimFunc(word, unicode.IsPunct))
	}
	first, last := words[0], words[len(words)-1]
	// Pronouns immediately before "thought" make it a verb, including after
	// introductory phrases ("Last night I thought"). An auxiliary ending gives
	// the same evidence in passive/perfect constructions ("It was thought").
	// An indefinite article directly before it introduces the ordinary noun
	// ("Here's a thought"), independently of the opening words.
	if personalPronoun(last) || thoughtAuxiliary(last) || indefiniteArticle(last) {
		return OrdinarySpeech
	}
	containsPronoun := false
	containsArticle := false
	for index, word := range words {
		containsPronoun = containsPronoun || personalPronoun(word)
		containsArticle = containsArticle || (index > 0 && indefiniteArticle(word))
	}
	if containsPronoun {
		// English -ly adverbs cover modifiers such as "suddenly" without an
		// expanding list. A few common irregular modifiers and noun articles
		// cover "I never thought" and "I had a thought".
		if strings.HasSuffix(last, "ly") {
			return OrdinarySpeech
		}
		switch last {
		case "just", "never", "always", "often", "once", "sometimes", "also":
			return OrdinarySpeech
		}
	}
	if cueDeterminer(first) {
		following := firstIntentWord(body)
		// A noun reading is strong in "My first thought was ..." and "The
		// first thought I ...". Longer titles followed by "I learned ..."
		// remain possible rather than being rejected for starting with "the".
		if thoughtAuxiliary(following) || (len(words) <= 2 && personalPronoun(following)) {
			return OrdinarySpeech
		}
		if len(words) == 1 {
			return OrdinarySpeech
		}
		return PossibleCommand
	}
	// An indefinite article inside a longer phrase is uncertain ("Just an
	// ordinary thought"). Comparative "more thought" is likewise not enough
	// evidence for an unknown Subject. These are grammatical positions, not a
	// growing list of conversational opening words.
	// Single words can modify the ordinary noun thought ("Random thought").
	// Sentence-like titles also need exact identity evidence; fuzzy similarity
	// and popularity must never turn these uncertain fragments into commands.
	if len(words) == 1 || containsPronoun || containsArticle || last == "more" || first == "remember" {
		return PossibleCommand
	}
	return ConfirmedCommand
}

func indefiniteArticle(word string) bool {
	switch word {
	case "a", "an", "another":
		return true
	}
	return false
}

func personalPronoun(word string) bool {
	word, _, _ = strings.Cut(strings.ReplaceAll(word, "’", "'"), "'")
	switch word {
	case "i", "we", "you", "he", "she", "it", "they":
		return true
	}
	return false
}

func thoughtAuxiliary(word string) bool {
	switch word {
	case "am", "is", "are", "was", "were", "be", "been", "being", "have", "has", "had", "do", "does", "did", "can", "could", "will", "would", "shall", "should", "may", "might", "must":
		return true
	}
	return false
}

func cueDeterminer(word string) bool {
	switch word {
	case "a", "an", "the", "my", "our", "your", "his", "her", "their", "this", "that", "these", "those", "another", "some":
		return true
	}
	return false
}

// Only a short lookahead at the body's first word informs noun constructions;
// recognition never scans/ranks the dictated body as a Subject.
func firstIntentWord(body string) string {
	characters := 0
	for offset, character := range body {
		if !unicode.IsLetter(character) {
			return strings.ToLower(body[:offset])
		}
		characters++
		if characters > maximumIntentWordCharacters {
			return ""
		}
	}
	return strings.ToLower(body)
}
