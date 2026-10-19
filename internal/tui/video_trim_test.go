package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/visual"
)

func TestModel_VideoTrim(t *testing.T) {
	t.Run("cut and source arrows nudge by a tenth of a second without rounding milliseconds", func(t *testing.T) {
		for _, focus := range []int{trimFocusSource, trimFocusStart, trimFocusEnd} {
			m := newRootTestModel()
			m.browserOptionsEnabled = true
			m.width, m.height = 80, 36
			m.options = optionsState{
				open:             true,
				trimming:         true,
				trimFocus:        focus,
				sourceDurationMS: 60000,
				sourcePositionMS: 1251,
				trim:             visual.VideoTrim{StartMS: 1251, EndMS: 1251},
			}
			position := func() int64 {
				switch focus {
				case trimFocusStart:
					return m.options.trim.StartMS
				case trimFocusEnd:
					return m.options.trim.EndMS
				default:
					return m.options.sourcePositionMS
				}
			}
			for _, action := range []struct {
				code rune
				mod  tea.KeyMod
				want int64
			}{
				{tea.KeyRight, 0, 1351}, {tea.KeyLeft, 0, 1251},
				{tea.KeyRight, tea.ModShift, 2251}, {tea.KeyLeft, tea.ModShift, 1251},
				{tea.KeyHome, 0, 0}, {tea.KeyLeft, 0, 0},
				{tea.KeyEnd, 0, 60000}, {tea.KeyRight, 0, 60000},
			} {
				m, _ = rootUpdate(m, tea.KeyPressMsg{Code: action.code, Mod: action.mod})
				if got := position(); got != action.want {
					t.Fatalf("focus %d key %d: got %d ms, want %d", focus, action.code, got, action.want)
				}
			}
			if !strings.Contains(m.View().Content, "←/→: 0.1s · Shift: 1s") {
				t.Fatal("help does not describe the arrow increments")
			}
		}
	})
	t.Run("unpreviewed cut edits preserve the accepted loop used by Done and Apply", func(t *testing.T) {
		m := newRootTestModel()
		m, _ = rootUpdate(m, BrowserOptionsEnabledMsg{})
		m, _ = rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 36})
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "open"})
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "loaded", ID: m.OptionsState().ID, Darkness: 70})
		m.activateOption()
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "staged", ID: m.OptionsState().ID, Kind: "video", DurationMS: 60000})
		if !m.OptionsState().Trimming || m.OptionsState().Trim.EndMS != 30000 {
			t.Fatalf("staged state = %+v", m.OptionsState())
		}
		m.options.trimFocus = trimFocusPreview
		m, _ = rootUpdate(m, enterKey())
		id := m.OptionsState().ID
		accepted := visual.VideoTrim{StartMS: 0, EndMS: 30000}
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "loopReady", ID: id, Trim: accepted, DurationMS: 30000})
		m.options.trimFocus = trimFocusStart
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyRight})
		if m.OptionsState().Trim.StartMS != 100 || m.options.acceptedTrim != accepted || !m.options.loopReady {
			t.Fatal("draft edit lost accepted loop")
		}
		if !strings.Contains(m.View().Content, "Previewed: 0.00–30.00s") {
			t.Fatal("accepted range not visible")
		}
		m.options.trimFocus = trimFocusDone
		m, _ = rootUpdate(m, enterKey())
		if m.OptionsState().Trimming {
			t.Fatal("Done did not return to Visual")
		}
		m.options.focus = optionsFocusApply
		m, _ = rootUpdate(m, enterKey())
		if m.OptionsState().Request != "apply" || m.options.acceptedTrim != accepted {
			t.Fatal("Apply did not retain accepted loop")
		}
	})
	t.Run("preview cells and separate controls fit normal and narrow terminals", func(t *testing.T) {
		for _, size := range [][2]int{{80, 36}, {60, 24}} {
			m := newRootTestModel()
			m.width, m.height = size[0], size[1]
			m.options = optionsState{
				open:             true,
				video:            true,
				speedPercent:     25,
				trimming:         true,
				sourceFrame:      true,
				sourceDurationMS: 60000,
				acceptedTrim:     visual.VideoTrim{EndMS: 30000},
				loopDurationMS:   30000,
				loopReady:        true,
			}
			rows := strings.Split(ansi.Strip(m.View().Content), "\n")
			if len(rows) > size[1] {
				t.Fatalf("%v: %d rows exceeds body", size, len(rows))
			}
			_, rect := m.optionsLayout()
			if !strings.Contains(rows[4], "╭") || !strings.Contains(rows[5], "│"+strings.Repeat(" ", rect.Width)+"│") {
				t.Fatalf("%v: preview is not below heading", size)
			}
			for _, label := range []string{"Preview loop", "Done — use previewed loop", "Cancel upload"} {
				found := false
				for _, row := range rows {
					if strings.TrimSpace(row) == label {
						found = true
					}
				}
				if !found {
					t.Fatalf("%v missing separate action %q", size, label)
				}
			}
			if !strings.Contains(m.View().Content, fmt.Sprintf("plays %.2fs", 120.0)) {
				t.Fatal("slowdown duration is not based on the accepted loop")
			}
		}
	})

	t.Run("Cancel remains clickable during preparation and retires its result", func(t *testing.T) {
		m := newRootTestModel()
		m.browserOptionsEnabled = true
		m.width, m.height = 80, 36
		m.options = optionsState{
			open:             true,
			id:               5,
			request:          "prepareLoop",
			video:            true,
			trimming:         true,
			sourceDurationMS: 60000,
		}
		m, _ = rootUpdate(m, tea.MouseClickMsg{X: 8, Y: 26, Button: tea.MouseLeft})
		if m.OptionsState().Request != "discardVideo" {
			t.Fatalf("cancel request = %q", m.OptionsState().Request)
		}
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "loopReady", ID: 5, Trim: visual.VideoTrim{EndMS: 1000}, DurationMS: 1000})
		if m.options.loopReady {
			t.Fatal("cancelled result accepted")
		}
	})

	t.Run("failed replacement choice and committed warning retire new-upload controls", func(t *testing.T) {
		for _, action := range []string{"failed", "warning"} {
			m := newRootTestModel()
			m.browserOptionsEnabled = true
			m.options = optionsState{
				open:      true,
				id:        7,
				request:   "choose",
				video:     true,
				newVideo:  true,
				loopReady: true,
			}
			if action == "warning" {
				m.options.request = "apply"
			}
			m, _ = rootUpdate(m, BrowserOptionsMsg{Action: action, ID: 7, Background: true, Kind: "video"})
			if m.options.newVideo || m.options.loopReady {
				t.Fatalf("%s retained new-upload controls", action)
			}
			if action == "warning" && !strings.Contains(m.View().Content, "Some background files could not be removed") {
				t.Fatal("cleanup warning does not cover temporary video files")
			}
		}
	})

	t.Run("invalid draft does not encode and stale prepared reply cannot publish", func(t *testing.T) {
		m := newRootTestModel()
		m.options = optionsState{
			open:             true,
			video:            true,
			trimming:         true,
			trimFocus:        trimFocusPreview,
			sourceDurationMS: 60000,
			trim:             visual.VideoTrim{StartMS: 30000, EndMS: 1000},
		}
		m.browserOptionsEnabled = true
		m, _ = rootUpdate(m, enterKey())
		if m.OptionsState().Request != "" {
			t.Fatal("invalid draft requested processing")
		}
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "loopReady", ID: 100, Trim: visual.VideoTrim{EndMS: 1000}, DurationMS: 1000})
		if m.options.loopReady {
			t.Fatal("stale result accepted")
		}
	})
}
