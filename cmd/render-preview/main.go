// render-preview prints a deterministic synthetic UI, not an interactive TUI.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/render"
)

const (
	curveColumn     = 12 // Ten timestamp columns, then a two-column gutter.
	curveWidth      = 10
	cardColumn      = curveColumn + curveWidth + 2
	selectedColor   = 62 // Xterm 256-color palette: purple selection, gray curves/rail.
	unselectedColor = 245
	baselineColor   = 240
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, output io.Writer) error {
	flags := flag.NewFlagSet("render-preview", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	width := flags.Int("width", 100, "terminal columns (at least 32)")
	height := flags.Int("height", 36, "terminal rows (at least 12)")
	scenario := flags.String("scenario", "main", "distributions, histogram, expanded, expanded-similar, main, adjacent, gapped, crowded, scale, or empty")
	selectedThought := flags.Int("selected-thought", 1, "selected histogram thought, numbered from 1; 0 means none")
	mode := flags.String("mode", "filled", "filled or outline")
	selected := flags.Int("selected", 3, "selected event, numbered from 1; 0 means none")
	exponent := flags.Float64("count-exponent", 0.8, "positive count exponent; 0.25 strongly boosts smaller counts")
	size := flags.Float64("size", 1, "distribution size from 0.5 to 1.25")
	plain := flags.Bool("plain", false, "omit ANSI colors")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			flags.SetOutput(output)
			flags.PrintDefaults()
			return nil
		}
		return err
	}
	if flags.NArg() != 0 || *width < 32 || *height < 12 || *selected < 0 || *selectedThought < 0 {
		return fmt.Errorf("use flags only, width >= 32, height >= 12, and selected >= 0")
	}
	if *exponent <= 0 || math.IsNaN(*exponent) || math.IsInf(*exponent, 0) || *size < 0.5 || *size > 1.25 || math.IsNaN(*size) {
		return fmt.Errorf("use a finite positive count-exponent and size between 0.5 and 1.25")
	}
	options := render.Options{CountExponent: *exponent, Size: *size}
	switch *mode {
	case "filled":
	case "outline":
		options.Mode = render.Outline
	default:
		return fmt.Errorf("mode must be filled or outline")
	}
	if *scenario == "histogram" {
		_, err := io.WriteString(output, drawHistogram(*width, *height, *selectedThought-1, options, *plain))
		return err
	}
	if *scenario == "distributions" {
		_, err := io.WriteString(output, drawDistributions(*width, *height, *selected-1, options, *plain))
		return err
	}
	scene, err := fixture(*scenario)
	if err != nil {
		return err
	}
	_, err = io.WriteString(output, draw(scene, *width, *height, *selected-1, *selectedThought-1, options, *plain))
	return err
}

func color(text string, code int, plain bool) string {
	if plain {
		return text
	}
	// SGR 38;5 selects a palette foreground; SGR 0 resets it after the text.
	return fmt.Sprintf("\x1b[38;5;%dm%s\x1b[0m", code, text)
}

func fit(text string, width int) string {
	text = ansi.Truncate(text, width, "…")
	return text + strings.Repeat(" ", max(0, width-ansi.StringWidth(text)))
}

func distributionRow(cells []render.Cell, selected int, plain bool) string {
	var line strings.Builder
	for _, cell := range cells {
		code := unselectedColor
		if cell.CurveIndex < 0 {
			code = baselineColor
		} else if cell.CurveIndex == selected {
			code = selectedColor
		}
		glyph := string(cell.Glyph)
		if cell.Glyph != ' ' {
			glyph = color(glyph, code, plain)
		}
		line.WriteString(glyph)
	}
	return line.String()
}

func draw(scene scene, width, height, selected, selectedThought int, options render.Options, plain bool) string {
	// Fixture row anchors are independent of curves, mode, selection, and width.
	// This deliberately is not a second implementation of Events layout.
	labels := make([]string, scene.endRow+1)
	right := make([]string, len(labels))
	for i := range right {
		right[i] = color("│", unselectedColor, plain)
	}
	for row, label := range scene.hours {
		labels[row] = label
	}
	for _, row := range scene.separators {
		right[row] = ""
	}
	for i, card := range scene.cards {
		if card.thoughts != nil {
			selected = i
		}
	}
	curves := make([]render.Distribution, 0, len(scene.cards))
	histogramTop, histogramEnd := -1, -1
	var histogram [][]render.Cell
	selectedCurve := -1
	for i, card := range scene.cards {
		count := fmt.Sprintf("%d thoughts", card.count)
		if card.count == 1 {
			count = "1 thought"
		}
		contents := []string{card.heading, count}
		if card.ongoing {
			contents = append(contents, "   Building  Break the next task into one small step.", "  Thought 31 • 12:24 PM")
		}
		if card.thoughts != nil {
			contents = []string{card.heading, "", count + " · Newest first"}
			for i, thought := range card.thoughts {
				if i > 0 {
					contents = append(contents, "")
				}
				title := "   Reading  " + thought.preview
				if i == selectedThought {
					title = color(title, selectedColor, plain)
				}
				contents = append(contents, title, thought.timestamp)
			}
		}
		cardHeight := len(contents) + 2 // Top and bottom borders.
		// Four Braille dot rows per terminal cell; center on the first/last dot midpoint.
		if card.thoughts == nil {
			if i == selected {
				selectedCurve = len(curves)
			}
			curves = append(curves, render.Distribution{CenterY: float64(card.top*4) + float64(cardHeight*4-1)/2, Count: card.count})
		} else {
			histogramTop, histogramEnd = card.top, card.top+cardHeight
			bars := make([]render.HistogramBar, 0, len(card.thoughts))
			var reference int64
			for i, thought := range card.thoughts {
				bars = append(bars, render.HistogramBar{Row: 4 + 3*i, Value: thought.characters})
				reference = max(reference, thought.characters)
			}
			histogram = render.RenderHistogram(curveWidth, cardHeight, bars, render.HistogramOptions{
				Mode:      options.Mode,
				Reference: reference,
				Exponent:  render.HistogramExponent(options.CountExponent),
				Size:      options.Size,
			})
		}
		labels[card.top], labels[card.top+cardHeight-1] = card.start, card.end
		borderColor := unselectedColor
		if i == selected || card.thoughts != nil {
			borderColor = selectedColor
		}
		innerWidth := width - cardColumn - 2
		right[card.top] = color("╭"+strings.Repeat("─", innerWidth)+"╮", borderColor, plain)
		for j, content := range contents {
			right[card.top+j+1] = color("│", borderColor, plain) + fit(content, innerWidth) + color("│", borderColor, plain)
		}
		right[card.top+cardHeight-1] = color("╰"+strings.Repeat("─", innerWidth)+"╯", borderColor, plain)
	}
	right[scene.endRow] = scene.ending
	// The ending marker is outside the drawing bounds, so curves cannot cover it.
	cells := render.RenderDistributions(curveWidth, scene.endRow, curves, options)
	body := make([]string, len(labels))
	for y := range body {
		lane := strings.Repeat(" ", curveWidth)
		if y >= histogramTop && y < histogramEnd {
			lane = distributionRow(histogram[y-histogramTop], selectedThought, plain)
		} else if y < len(cells) {
			lane = distributionRow(cells[y], selectedCurve, plain)
		}
		body[y] = fit(labels[y], 10) + "  " + lane + "  " + right[y]
	}
	lines := []string{"Local user: local (demo)", "", " Events  " + scene.date, ""}
	bodyHeight := height - 9 // Header plus status/help/entity footer remain visible.
	offset := max(0, len(body)-bodyHeight)
	lines = append(lines, body[offset:]...)
	for len(lines) < 4+bodyHeight {
		lines = append(lines, "")
	}
	help := "← day / → open • Home/End: first/now • r: refresh • n: start • e: end • q: quit"
	if scene.ending == "..." {
		help = "Home: first • End: now • ←/→: day • r: refresh • n: start • e: end • q: quit"
	}
	if histogram != nil {
		help = "← collapse • → open • ↑/↓ thoughts • PgUp/PgDn scroll • d distributions"
	}
	lines = append(lines, "", help, strings.Repeat("─", width), "Thoughts   Subjects", "Tab: panel • ←/→: entity • Enter: open")
	for i := range lines {
		lines[i] = fit(lines[i], width)
	}
	return strings.Join(lines, "\n") + "\n"
}
