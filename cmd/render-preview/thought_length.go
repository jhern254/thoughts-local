package main

import (
	"fmt"
	"strings"

	"github.com/jhern254/go-thoughts/internal/render"
)

type previewThought struct {
	preview    string
	timestamp  string
	characters int64
}

func previewThoughts() []previewThought {
	return []previewThought{
		{preview: "A longer reflection on the chapter…", timestamp: "  Thought 43 • Oct 4, 2026 9:50 AM PDT", characters: 320},
		{preview: "Compare these two explanations…", timestamp: "  Thought 42 • Oct 4, 2026 9:30 AM PDT", characters: 90},
		{preview: "Revisit this passage.", timestamp: "  Thought 41 • Oct 4, 2026 9:10 AM PDT", characters: 20},
	}
}

func drawThoughtLengths(width, height, selected int, options render.Options, plain bool) string {
	thoughts := previewThoughts()
	bars := make([]render.ThoughtLengthBar, len(thoughts))
	for i, thought := range thoughts {
		bars[i] = render.ThoughtLengthBar{Row: 3 * i, CharacterCount: thought.characters}
	}
	cells := render.RenderThoughtLengthBars(curveWidth, 8, bars, render.ThoughtLengthBarOptions{
		Mode:      options.Mode,
		Reference: 320,
		Exponent:  render.ThoughtLengthExponent(options.CountExponent),
		Size:      options.Size,
	})
	lines := []string{"Thought length bars", "Whole-event maximum: 320 characters", ""}
	for y, row := range cells {
		text := distributionRow(row, selected, plain)
		if y%3 == 0 {
			text += fmt.Sprintf("  %d characters", thoughts[y/3].characters)
		}
		lines = append(lines, text)
	}
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i := range lines {
		lines[i] = fit(lines[i], width)
	}
	return strings.Join(lines[:height], "\n") + "\n"
}
