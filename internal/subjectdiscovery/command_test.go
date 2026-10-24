package subjectdiscovery

import (
	"strings"
	"testing"
)

func TestParseSpokenPrefix(t *testing.T) {
	for _, tc := range []struct{ name, text, cue, body string }{
		{"plural command", "Fitness thoughts: try shorter workouts", "Fitness", "try shorter workouts"},
		{"plural unicode and case", "日本語 学習 ThOuGhTs：今日は学んだ", "日本語 学習", "今日は学んだ"},
		{"plural punctuation", "C++ & Go thoughts—review pointers", "C++ & Go", "review pointers"},
		{"plural at end", "Fitness THOUGHTS", "Fitness", ""},
		{"plural possessive noun", "My thoughts are scattered", "", ""},
		{"plural demonstrative noun", "These thoughts are useful", "", ""},
		{"plural without cue", "thoughts", "", ""},
		{"embedded plural", "Fitness afterthoughts: exercise", "", ""},
		{"possessive plural", "Fitness thoughts’ meaning", "", ""},
		{"ASCII possessive", "Fitness thought's meaning", "", ""},
		{"first plural wins", "Fitness thoughts: Go thought exercise", "Fitness", "Go thought exercise"},
		{"first singular wins", "Fitness thought: Go thoughts exercise", "Fitness", "Go thoughts exercise"},
		{"ordinary first plural stops scanning", "My thoughts are scattered Fitness thought exercise", "", ""},
		{"ordinary first singular stops scanning", "I thought Fitness thoughts exercise", "", ""},
		{"missing first cue stops scanning", "thoughts Fitness thought exercise", "", ""},
		{"alias", "Programming in Go thought, today I learned...", "Programming in Go", "today I learned..."},
		{"canonical", "Go Programming thought I learned about interfaces", "Go Programming", "I learned about interfaces"},
		{"unicode", "日本語 学習 THOUGHT：今日は学んだ", "日本語 学習", "今日は学んだ"},
		{"punctuation", "C++ & Go thought—review pointers", "C++ & Go", "review pointers"},
		{"partial command", "Go Programming thought", "Go Programming", ""},
		{"personal sentence", "I thought this was interesting", "", ""},
		{"conversational noun", "Just a thought about work.", "", ""},
		{"presenting a thought", "Here's a thought for tomorrow.", "", ""},
		{"presenting another thought", "Here's another thought about Go.", "", ""},
		{"unicode conversational punctuation", "Here’s another THOUGHT：about Go.", "", ""},
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

	t.Run("both spellings preserve cue bounds and original byte offsets", func(t *testing.T) {
		for _, delimiter := range []string{"thought", "thoughts"} {
			for _, scenario := range []struct {
				name, cue string
				accepted  bool
			}{
				{"Unicode cue", "Études 日本語", true},
				{"eight words", "one two three four five six seven eight", true},
				{"nine words", "one two three four five six seven eight nine", false},
				{"255 characters", strings.Repeat("界", 255), true},
				{"256 characters", strings.Repeat("界", 256), false},
			} {
				t.Run(delimiter+"/"+scenario.name, func(t *testing.T) {
					originalPrefix := scenario.cue + " " + delimiter + "：  "
					text := originalPrefix + "étudier."
					prefix, recognized := ParseSpokenPrefix(text)
					if recognized != scenario.accepted {
						t.Fatalf("recognized = %v, want %v", recognized, scenario.accepted)
					}
					if recognized && (prefix.Cue != scenario.cue || prefix.BodyStart != len(originalPrefix)) {
						t.Fatalf("prefix = %+v, want cue %q and byte offset %d", prefix, scenario.cue, len(originalPrefix))
					}
				})
			}
		}
	})
}

func TestSpokenPrefix_Intent(t *testing.T) {
	for _, tc := range []struct {
		text string
		want CommandIntent
	}{
		{"Fitness thoughts: try shorter workouts", ConfirmedCommand},
		{"Random thoughts…", PossibleCommand},
		{"My thoughts are scattered", OrdinarySpeech},
		{"These thoughts are useful", OrdinarySpeech},
		{"Go Programming thought, today I learned.", ConfirmedCommand},
		{"Programming in Go thought studied goroutines.", ConfirmedCommand},
		{"Machine Learning Research thought, studied embeddings.", ConfirmedCommand},
		{"go programming practice thought, reviewed concurrency.", ConfirmedCommand},
		{"日本語 学習 THOUGHT：勉強した", ConfirmedCommand},
		{"C++ & Go thought—review pointers", ConfirmedCommand},
		{"Go Programming thought", ConfirmedCommand},
		{"Random thought, perhaps I should learn Go.", PossibleCommand},
		{"One more thought about my goals.", PossibleCommand},
		{"Just an ordinary thought about today.", PossibleCommand},
		{"Fitness thought", ConfirmedCommand},
		{"fitness thought I’m trying to start", ConfirmedCommand},
		{"meditation thought, practice today", ConfirmedCommand},
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
