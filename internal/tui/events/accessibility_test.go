package events

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
)

func TestModel_BorderWeight(t *testing.T) {
	t.Run("changes only the border glyphs and preserves focus styling", func(t *testing.T) {
		m, _, _ := fixture(t)
		content := []string{"Example event", "Subject time"}
		before := m.paintCard(content, true)
		m.SetBorderWeight(data.BorderHeavy)
		after := m.paintCard(content, true)
		if len(before) != len(after) {
			t.Fatal("border weight changed card height")
		}
		for i, row := range after {
			if got, want := ansi.StringWidth(row), ansi.StringWidth(before[i]); got != want {
				t.Fatalf("row %d width: got %d, want %d", i, got, want)
			}
		}
		if !strings.Contains(after[0], "┏") || !strings.Contains(before[0], "╭") {
			t.Fatal("border selection did not change glyphs")
		}
		if after[0] == ansi.Strip(after[0]) {
			t.Fatal("focused border lost highlight")
		}
		for i := 1; i < len(after)-1; i++ {
			width := ansi.StringWidth(after[i])
			if ansi.Cut(ansi.Strip(after[i]), 1, width-1) != ansi.Cut(ansi.Strip(before[i]), 1, width-1) {
				t.Fatal("border weight changed card content")
			}
		}
		m.SetFocused(false)
		blurred := m.paintCard(content, true)
		if blurred[0] != ansi.Strip(blurred[0]) || !strings.Contains(blurred[0], "┏") {
			t.Fatal("blur did not preserve thick unhighlighted border")
		}
	})
}
