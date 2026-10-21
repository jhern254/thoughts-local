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
		{"narrative", "I had a thought about work", "", ""},
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
