package main

import (
	"fmt"
	"strings"

	"github.com/jhern254/go-thoughts/internal/render"
)

func drawDayActivity(width int, options render.Options, plain bool) string {
	values := []render.DayActivity{{}, {Events: 1}, {Events: 4, Thoughts: 10}, {Events: 2, Thoughts: 80}, {}, {Thoughts: 20}, {Events: 3, Thoughts: 5}, {Events: 7, Thoughts: 40}, {Events: 5, Thoughts: 10}, {Events: 1}, {}}
	var out strings.Builder
	out.WriteString(" Mindset  Practice deliberately.\n\n")
	for _, focused := range []bool{false, true} {
		out.WriteByte('\n')
		cells := render.RenderDayActivity(width, 3, values, render.DayActivityOptions{Total: true, Events: focused, Thoughts: focused, Outline: options.Mode == render.Outline, Exponent: options.CountExponent, Size: options.Size})
		for _, row := range cells {
			for _, cell := range row {
				out.WriteString(color(string(cell.Glyph), [...]int{240, 37, 179}[cell.Series], plain))
			}
			out.WriteByte('\n')
		}
		out.WriteString(strings.Repeat("─", width/2) + color("┬", 62, plain) + strings.Repeat("─", width-width/2-1) + "\n")
		fmt.Fprintln(&out, " Events  October 16, 2026")
		if focused {
			out.WriteString(fit("1 Total · 2 Events · 3 Thoughts · +/- zoom · d style", width) + "\n")
		}
		out.WriteByte('\n')
	}
	return out.String()
}
