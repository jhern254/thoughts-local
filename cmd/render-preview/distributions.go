package main

import (
	"strings"

	"github.com/jhern254/go-thoughts/internal/render"
)

// The same counts are used in each column; only their vertical centers change.
// This isolates Gaussian superposition from any future event-to-time mapping.
func drawDistributions(width, height, selected int, options render.Options, plain bool) string {
	groups := []struct {
		name    string
		centers [3]float64
	}{
		{"Separated", [3]float64{19.5, 55.5, 83.5}},
		{"Shared valleys", [3]float64{23.5, 43.5, 71.5}},
		{"Merged peak", [3]float64{27.5, 35.5, 71.5}},
	}
	columnWidth := (width - 4) / 3
	laneWidth := min(curveWidth, columnWidth)
	leftPadding := strings.Repeat(" ", (columnWidth-laneWidth)/2)
	plotHeight := height - 5
	plots := make([][][]render.Cell, len(groups))
	var headings []string
	for i, group := range groups {
		curves := []render.Distribution{
			{CenterY: group.centers[0], Count: 20},
			{CenterY: group.centers[1], Count: 20},
			{CenterY: group.centers[2], Count: 0},
		}
		plots[i] = render.RenderDistributions(laneWidth, plotHeight, curves, options)
		headings = append(headings, fit(group.name, columnWidth))
	}
	lines := []string{"Connected Gaussian distributions", "Same counts: 20 + 20 + 0; different distances between centers.", strings.Join(headings, "  "), ""}
	for y := range plotHeight {
		var columns []string
		for _, plot := range plots {
			columns = append(columns, fit(leftPadding+distributionRow(plot[y], selected, plain), columnWidth))
		}
		lines = append(lines, strings.Join(columns, "  "))
	}
	lines = append(lines, "Purple: selected contribution. Gray: others. Shapes share one continuous profile.")
	for i := range lines {
		lines[i] = fit(lines[i], width)
	}
	return strings.Join(lines, "\n") + "\n"
}
