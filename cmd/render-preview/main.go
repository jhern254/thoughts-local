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
	selectedColor   = 62
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
	scenario := flags.String("scenario", "main", "distributions, main, adjacent, gapped, crowded, scale, or empty")
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
	if flags.NArg() != 0 || *width < 32 || *height < 12 || *selected < 0 {
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
	if *scenario == "distributions" {
		_, err := io.WriteString(output, drawDistributions(*width, *height, *selected-1, options, *plain))
		return err
	}
	scene, err := fixture(*scenario)
	if err != nil {
		return err
	}
	_, err = io.WriteString(output, draw(scene, *width, *height, *selected-1, options, *plain))
	return err
}

func color(text string, code int, plain bool) string {
	if plain {
		return text
	}
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

func draw(scene scene, width, height, selected int, options render.Options, plain bool) string {
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
	curves := make([]render.Distribution, 0, len(scene.cards))
	for i, card := range scene.cards {
		count := fmt.Sprintf("%d thoughts", card.count)
		if card.count == 1 {
			count = "1 thought"
		}
		contents := []string{card.heading, count}
		if card.ongoing {
			contents = append(contents, "   Building  Break the next task into one small step.", "  Thought 31 • 12:24 PM")
		}
		cardHeight := len(contents) + 2
		curves = append(curves, render.Distribution{CenterY: float64(card.top*4) + float64(cardHeight*4-1)/2, Count: card.count})
		labels[card.top], labels[card.top+cardHeight-1] = card.start, card.end
		borderColor := unselectedColor
		if i == selected {
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
		if y < len(cells) {
			lane = distributionRow(cells[y], selected, plain)
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
	lines = append(lines, "", help, strings.Repeat("─", width), "Thoughts   Subjects", "Tab: panel • ←/→: entity • Enter: open")
	for i := range lines {
		lines[i] = fit(lines[i], width)
	}
	return strings.Join(lines, "\n") + "\n"
}
