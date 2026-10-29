//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
)

// Exercise the same native renderer as Bubble Tea, with no async model or
// browser resize ordering involved. The browser replays these actual ANSI bytes.
func rendererResizeFrames(t *testing.T) string {
	t.Helper()
	type frame struct {
		Columns int    `json:"cols"`
		Rows    int    `json:"rows"`
		Output  string `json:"output"`
	}
	var output bytes.Buffer
	renderer := uv.NewTerminalRenderer(&output, []string{"TERM=xterm-256color"})
	renderer.SetColorProfile(colorprofile.TrueColor)
	renderer.SetFullscreen(true)
	renderer.EnterAltScreen()
	var frames []frame
	for _, state := range []struct{ width, height, top int }{{55, 26, 10}, {111, 21, 10}, {111, 21, 4}} {
		lines := make([]string, state.height)
		lines[0] = "Synthetic renderer resize"
		for row := state.top; row < state.top+4; row++ {
			lines[row] = strings.Repeat(" ", state.width-35) + "\x1b[38;5;240m" + strings.Repeat("⣿", 35) + "\x1b[m"
		}
		lines[state.top+4] = strings.Repeat(" ", state.width-1) + "\x1b[48;5;62m \x1b[m"
		lines[state.top+5] = " Events — synthetic card below the mountains"
		lines[state.top+7] = "                        │ Event card │"
		buffer := uv.NewScreenBuffer(state.width, state.height)
		uv.NewStyledString(strings.Join(lines, "\n")).Draw(buffer, buffer.Bounds())
		if len(frames) < 2 {
			renderer.Resize(state.width, state.height)
			renderer.Erase()
		}
		renderer.Render(buffer.RenderBuffer)
		if err := renderer.Flush(); err != nil {
			t.Fatal(err)
		}
		frames = append(frames, frame{state.width, state.height, strings.Clone(output.String())})
		output.Reset()
	}
	body, err := json.Marshal(frames)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "renderer-resize.json")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
