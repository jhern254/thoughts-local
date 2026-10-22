package subjectdiscovery

import (
	"strings"
	"testing"
)

func TestParseSpokenPrefix(t *testing.T) {
	for _, tc := range []struct{ name, text, cue, body string }{
		{"alias", "Programming in Go thought, today I learned...", "Programming in Go", "today I learned..."},
		{"canonical", "Go Programming thought I learned about interfaces", "Go Programming", "I learned about interfaces"},
		{"unicode", "日本語 学習 THOUGHT：今日は学んだ", "日本語 学習", "今日は学んだ"},
		{"punctuation", "C++ & Go thought—review pointers", "C++ & Go", "review pointers"},
		{"partial command", "Go Programming thought", "Go Programming", ""},
		{"personal sentence", "I thought this was interesting", "", ""},
		{"introductory time", "Yesterday I thought about Go.", "", ""},
		{"introductory time phrase", "Last night I thought about work.", "", ""},
		{"possessive noun", "My first thought was to practice.", "", ""},
		{"passive", "It was thought to be finished.", "", ""},
		{"pronoun and adverb", "I suddenly thought about something.", "", ""},
		{"introductory preposition", "For a moment I thought it worked.", "", ""},
		{"today narrative", "Today I thought about my goals.", "", ""},
		{"plural pronoun", "Earlier we thought about moving.", "", ""},
		{"determiner noun", "The first thought I had was interesting.", "", ""},
		{"emphasis", "I really thought this would work.", "", ""},
		{"introductory punctuation", "Yesterday, I THOUGHT：about Go.", "", ""},
		{"unicode punctuation", "Earlier — we thought，about moving.", "", ""},
		{"narrative", "I had a thought about work", "", ""},
		{"article-led title", "The Art of Learning thought, practice recall", "The Art of Learning", "practice recall"},
		{"indefinite article title", "A Better Tomorrow thought plan ahead", "A Better Tomorrow", "plan ahead"},
		{"pronoun-like title", "I Ching thought read chapter two", "I Ching", "read chapter two"},
		{"sentence title", "I Want to Learn Go thought, review interfaces", "I Want to Learn Go", "review interfaces"},
		{"possessive title", "My Learning Journal thought review notes", "My Learning Journal", "review notes"},
		{"demonstrative title", "This Is Water thought review the essay", "This Is Water", "review the essay"},
		{"imperative title", "Remember Tomorrow thought make a plan", "Remember Tomorrow", "make a plan"},
		{"narrative with modifier", "I never thought about work", "", ""},
		{"narrative with auxiliary", "we had thought about work", "", ""},
		{"narrative with longer prefix", "I also had a thought about work", "", ""},
		{"article alone", "the thought was interesting", "", ""},
		{"pronoun alone", "they thought about work", "", ""},
		{"no delimiter", "Remember to study Go", "", ""},
		{"no cue", "Thought, remember tomorrow", "", ""},
		{"not standalone", "Work thoughtful notes", "", ""},
		{"possessive", "Work thought’s meaning", "", ""},
		{"partial delimiter", "Programming in Go thou", "", ""},
		{"too many words", "one two three four five six seven eight nine thought body", "", ""},
		{"too long", strings.Repeat("界", 256) + " thought body", "", ""},
		{"boundary length", strings.Repeat("界", 255) + " thought body", strings.Repeat("界", 255), "body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseSpokenPrefix(tc.text)
			if tc.cue == "" {
				if ok {
					t.Fatalf("got %+v, want no command", got)
				}
				return
			}
			if !ok || got.Cue != tc.cue || tc.text[got.BodyStart:] != tc.body {
				t.Fatalf("got %+v/%v, want cue %q and body %q", got, ok, tc.cue, tc.body)
			}
		})
	}
}

func TestSpokenPrefix_Intent(t *testing.T) {
	for _, tc := range []struct {
		text string
		want CommandIntent
	}{
		{"Go Programming thought, today I learned.", ConfirmedCommand},
		{"Programming in Go thought studied goroutines.", ConfirmedCommand},
		{"Machine Learning Research thought, studied embeddings.", ConfirmedCommand},
		{"go programming practice thought, reviewed concurrency.", ConfirmedCommand},
		{"日本語 学習 THOUGHT：勉強した", ConfirmedCommand},
		{"C++ & Go thought—review pointers", ConfirmedCommand},
		{"Go Programming thought", ConfirmedCommand},
		{"Random thought, perhaps I should learn Go.", PossibleCommand},
		{"Fitness thought", PossibleCommand},
		{"The Art of Learning thought, review notes.", PossibleCommand},
		{"I Ching thought, review notes.", PossibleCommand},
		{"I Want to Learn Go thought, review interfaces.", PossibleCommand},
		{"My Learning Journal thought, review notes.", PossibleCommand},
		{"Remember Tomorrow thought, review notes.", PossibleCommand},
		{"Yesterday I thought about Go.", OrdinarySpeech},
		{"It was thought to be finished.", OrdinarySpeech},
		{"The first thought I had was interesting.", OrdinarySpeech},
	} {
		t.Run(tc.text, func(t *testing.T) {
			got, _ := ParseSpokenPrefix(tc.text)
			if got.Intent != tc.want {
				t.Fatalf("intent=%v, want %v", got.Intent, tc.want)
			}
		})
	}
}
