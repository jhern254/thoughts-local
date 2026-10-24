package voice

import (
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/speech"
)

func TestFormatTranscript(t *testing.T) {
	for _, tc := range []struct {
		name           string
		gap, threshold time.Duration
		want           string
	}{
		{"short pause", 1999 * time.Millisecond, 2 * time.Second, "first second"},
		{"threshold pause", 2 * time.Second, 2 * time.Second, "first\nsecond"},
		{"long pause", 5 * time.Second, 2 * time.Second, "first\nsecond"},
		{"disabled", 5 * time.Second, 0, "first second"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			transcript := speech.Transcript{Text: "first second", Segments: []speech.Segment{{ID: 1, Text: "first", Start: 0, Duration: time.Second, Complete: true}, {ID: 2, Text: "second", Start: time.Second + tc.gap, Duration: time.Second}}}
			formatter := TranscriptFormatter{Pause: tc.threshold}
			if got := formatter.Format(transcript); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
	t.Run("missing timing and incomplete phrases do not invent pauses", func(t *testing.T) {
		for _, segment := range []speech.Segment{{Text: "first"}, {Text: "first", Duration: time.Second}} {
			transcript := speech.Transcript{Text: "first second", Segments: []speech.Segment{segment, {Text: "second", Start: 5 * time.Second, Duration: time.Second}}}
			formatter := TranscriptFormatter{Pause: time.Second}
			if got := formatter.Format(transcript); got != "first second" {
				t.Fatalf("got %q", got)
			}
		}
	})
}

func TestTranscriptFormatter_Revisions(t *testing.T) {
	t.Run("keeps a chosen break while the current phrase timing changes", func(t *testing.T) {
		formatter := TranscriptFormatter{Pause: 2 * time.Second}
		transcript := speech.Transcript{Text: "first second", Segments: []speech.Segment{{ID: 1, Text: "first", Duration: time.Second, Complete: true}, {ID: 2, Text: "second", Start: 4 * time.Second, Duration: time.Second}}}
		if got := formatter.Format(transcript); got != "first\nsecond" {
			t.Fatalf("got %q", got)
		}
		transcript.Segments[1].Start = 2 * time.Second
		transcript.Segments[1].Text = "second revised"
		if got := formatter.Format(transcript); got != "first\nsecond revised" {
			t.Fatalf("got %q, want stable break", got)
		}
	})
}
