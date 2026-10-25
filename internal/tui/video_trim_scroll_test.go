package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/visual"
)

func TestModel_VideoTrimScrolling(t *testing.T) {
	t.Run("resizing preserves the full preview while focus scrolls controls", func(t *testing.T) {
		m := newRootTestModel()
		m.width, m.height = 100, 50
		cut := visual.VideoTrim{StartMS: 1000, EndMS: 3000}
		m.options = optionsState{open: true, trimming: true, id: 5, sourceDurationMS: 60000, trim: cut, trimFocus: trimFocusSource}
		preview := m.OptionsState().Preview
		m, _ = rootUpdate(m, tea.WindowSizeMsg{Width: 60, Height: 34})
		if got := m.OptionsState().Preview.Height; got != preview.Height {
			t.Fatalf("small preview height: got %d, want %d", got, preview.Height)
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
		if m.options.trimFocus != trimFocusCancel {
			t.Fatalf("focus=%d, want Cancel", m.options.trimFocus)
		}
		rows := strings.Split(ansi.Strip(m.View().Content), "\n")
		if len(rows) > 34 {
			t.Fatalf("rendered rows=%d, want <=34", len(rows))
		}
		cancelRow := -1
		for row, text := range rows {
			if strings.TrimSpace(text) == "Cancel upload" {
				cancelRow = row
			}
		}
		if cancelRow < preview.Y+preview.Height {
			t.Fatalf("Cancel row=%d, want visible beneath preview", cancelRow)
		}
		if strings.Contains(strings.Join(rows, "\n"), "Source / Start:") {
			t.Fatal("control viewport did not scroll to Cancel")
		}
		if m.options.trim != cut || m.options.id != 5 {
			t.Fatal("resizing or scrolling changed cut/operation")
		}
		m, _ = rootUpdate(m, tea.WindowSizeMsg{Width: 100, Height: 50})
		if m.OptionsState().Preview.Height != preview.Height || m.options.trimFocus != trimFocusCancel {
			t.Fatal("growing window lost preview size/focus")
		}
	})
	t.Run("render completion keeps Preview and Done at the same rows", func(t *testing.T) {
		m := newRootTestModel()
		m.width, m.height = 60, 34
		m.browserOptionsEnabled = true
		cut := visual.VideoTrim{EndMS: 1000}
		m.options = optionsState{open: true, trimming: true, id: 5, request: "prepareLoop", sourceDurationMS: 60000, trim: cut, trimFocus: trimFocusPreview}
		rowOf := func(label string) int {
			for row, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
				if strings.TrimSpace(line) == label {
					return row
				}
			}
			t.Fatalf("missing %q", label)
			return -1
		}
		stop := rowOf("Stop rendering")
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "loopReady", ID: 5, Trim: cut, DurationMS: 1000, VideoQuality: visual.VideoQualityEfficient, Width: 96, Height: 64, FrameRate: visual.VideoFrameRate{Numerator: 24, Denominator: 1}})
		if got := rowOf("Preview loop"); got != stop {
			t.Fatalf("Preview row=%d, want Stop row %d", got, stop)
		}
		if got := rowOf("Done — use previewed loop"); got != stop+1 {
			t.Fatalf("Done row=%d, want %d", got, stop+1)
		}
	})
	t.Run("wheel focus reveals Cancel and clicks use scrolled rows", func(t *testing.T) {
		m := newRootTestModel()
		m.width, m.height = 60, 34
		m.options = optionsState{open: true, trimming: true, id: 5, sourceDurationMS: 60000, trim: visual.VideoTrim{EndMS: 30000}}
		for range 5 {
			m, _ = rootUpdate(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown, Y: 20})
		}
		if m.options.trimFocus != trimFocusCancel {
			t.Fatalf("wheel focus=%d, want Cancel", m.options.trimFocus)
		}
		row := -1
		for i, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
			if strings.TrimSpace(line) == "Cancel upload" {
				row = i
			}
		}
		if row < 0 {
			t.Fatal("wheel did not reveal Cancel")
		}
		m, _ = rootUpdate(m, tea.MouseClickMsg{X: 6, Y: row, Button: tea.MouseLeft})
		if m.options.request != "discardVideo" || m.options.id != 6 {
			t.Fatalf("scrolled click: request=%q id=%d", m.options.request, m.options.id)
		}
	})
	t.Run("undersized rendering retains Stop and Cancel ownership", func(t *testing.T) {
		m := newRootTestModel()
		m.width, m.height = 40, 20
		cut := visual.VideoTrim{EndMS: 30000}
		m.options = optionsState{open: true, trimming: true, id: 5, request: "prepareLoop", trim: cut}
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyEscape})
		if m.options.request != "stopPreview" || m.options.id != 6 || m.options.trim != cut {
			t.Fatal("small-window Stop lost source/cut ownership")
		}
		m, _ = rootUpdate(m, enterKey())
		if m.options.request != "discardVideo" || m.options.id != 7 {
			t.Fatal("small-window Cancel was unavailable while Stop joined")
		}
	})
	t.Run("undersized windows preserve the cut and block hidden editing", func(t *testing.T) {
		m := newRootTestModel()
		m.width, m.height = 40, 20
		cut := visual.VideoTrim{StartMS: 1000, EndMS: 3000}
		m.options = optionsState{open: true, trimming: true, id: 5, sourceDurationMS: 60000, trim: cut}
		if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Resize") || strings.Contains(view, "Source / Start:") {
			t.Fatalf("undersized view=%q", view)
		}
		if m.OptionsState().Preview.Height != 0 {
			t.Fatal("undersized view published a clipped preview")
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyRight})
		if m.options.trim != cut || m.options.id != 5 {
			t.Fatal("hidden control accepted an edit")
		}
		m, _ = rootUpdate(m, tea.WindowSizeMsg{Width: 60, Height: 34})
		if m.OptionsState().Preview.Height != 12 || m.options.trim != cut {
			t.Fatal("resizing did not restore full preview and cut")
		}
	})
}

func TestModel_VideoTrimStageLayout(t *testing.T) {
	t.Run("validation and prepared stages share control and footer rows", func(t *testing.T) {
		m := newRootTestModel()
		m.width, m.height = 100, 50
		m.options = optionsState{open: true, sourceChecking: true, request: "choose", trimFocus: trimFocusCancel}
		rowOf := func(label string) int {
			t.Helper()
			for row, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
				if strings.Contains(line, label) {
					return row
				}
			}
			t.Fatalf("missing %q", label)
			return -1
		}
		labels := []string{"Source / Start:", "End:", "Selected:", "Video quality:", "Playback speed:", "Preview loop", "Done — use previewed loop", "Cancel upload"}
		rows := make([]int, len(labels))
		for i, label := range labels {
			rows[i] = rowOf(label)
		}
		footer := rowOf("No prepared loop yet.")
		preparation := m.OptionsState().Preparation
		m.options.sourceChecking, m.options.trimming = false, true
		m.options.request = "prepareLoop"
		m.options.trimFocus = trimFocusPreview
		m.options.sourceDurationMS = 60000
		m.options.trim = visual.VideoTrim{EndMS: 1000}
		if got := rowOf("Stop rendering"); got != rows[5] {
			t.Fatalf("Stop row=%d, want %d", got, rows[5])
		}
		if m.videoTrimControlEnabled(trimFocusDone) {
			t.Fatal("Done enabled during rendering")
		}
		if got := m.OptionsState().Preparation; got != preparation {
			t.Fatalf("rendering rectangle=%+v, want %+v", got, preparation)
		}
		m.options.request, m.options.loopReady = "", true
		m.options.status = "Preview ready. Done uses this loop."
		for i, label := range labels {
			if got := rowOf(label); got != rows[i] {
				t.Fatalf("%s row=%d, want %d", label, got, rows[i])
			}
		}
		if got := rowOf("Previewed:"); got != footer {
			t.Fatalf("prepared footer=%d, want validation footer %d", got, footer)
		}
		view := strings.Split(ansi.Strip(m.View().Content), "\n")
		if strings.TrimSpace(view[rowOf("Preview ready.")-1]) != "" {
			t.Fatal("missing blank row above Preview ready")
		}
	})
}
