package voice

import (
	"strings"
	"time"

	"github.com/jhern254/go-thoughts/internal/speech"
)

// TranscriptFormatter belongs to one recording. It uses audio boundaries, never delays between
// inference replies. A completed phrase cannot move as partial text is revised.
// No trailing break is inserted until the next phrase actually has text.
type TranscriptFormatter struct {
	Pause  time.Duration
	breaks map[uint64]bool
}

func (formatter *TranscriptFormatter) Format(transcript speech.Transcript) string {
	pause := formatter.Pause
	if pause <= 0 || len(transcript.Segments) == 0 {
		return transcript.Text
	}
	var text strings.Builder
	var previous speech.Segment
	for _, segment := range transcript.Segments {
		if segment.Text == "" {
			continue
		}
		if text.Len() > 0 {
			separator := byte(' ')
			// Subtraction avoids overflow when adding untrusted durations to offsets.
			if previous.Complete && previous.Start >= 0 && previous.Duration > 0 && segment.Duration > 0 && segment.Start >= previous.Start {
				gap := segment.Start - previous.Start
				if formatter.breaks == nil {
					formatter.breaks = make(map[uint64]bool)
				}
				newline, known := formatter.breaks[segment.ID]
				if !known {
					newline = gap >= previous.Duration && gap-previous.Duration >= pause
					formatter.breaks[segment.ID] = newline
				}
				if newline {
					separator = '\n'
				}
			}
			text.WriteByte(separator)
		}
		text.WriteString(segment.Text)
		previous = segment
	}
	return text.String()
}
