package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/visual"
)

// Behavior fixtures use a full editor; resize/scrolling cases set their own dimensions.
func newVideoTrimTestModel() Model {
	model := newRootTestModel()
	model.width, model.height = 100, 50
	return model
}

func TestModel_VideoTrim(t *testing.T) {
	t.Run("selection help advertises MKV without claiming H264 is required", func(t *testing.T) {
		m := newVideoTrimTestModel()
		m.browserOptionsEnabled = true
		m.width, m.height = 80, 50
		m.options = optionsState{open: true, request: "load", id: 5}
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "loaded", ID: 5})
		if !strings.Contains(ansi.Strip(m.View().Content), "MKV") {
			t.Fatalf("format help=%q, want MKV", m.options.status)
		}
	})

	t.Run("matching prepared loop disables Preview and skips it during navigation", func(t *testing.T) {
		m := newVideoTrimTestModel()
		m.browserOptionsEnabled = true
		m.width, m.height = 80, 50
		trim := visual.VideoTrim{EndMS: 30000}
		m.options = optionsState{open: true, trimming: true, trimFocus: trimFocusPreview, sourceDurationMS: 60000, trim: trim, videoQuality: visual.VideoQualityEfficient}
		m, _ = rootUpdate(m, enterKey())
		requestID := m.OptionsState().ID
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "loopReady", ID: requestID, Trim: trim, DurationMS: 30000, VideoQuality: visual.VideoQualityEfficient, Width: 1280, Height: 720, FrameRate: visual.VideoFrameRate{Numerator: 24, Denominator: 1}})
		if m.options.trimFocus != trimFocusDone {
			t.Fatal("completed preview did not move focus to Done")
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
		if m.options.trimFocus != trimFocusSpeed {
			t.Fatal("reverse Tab focused an unchanged Preview")
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyTab})
		if m.options.trimFocus != trimFocusDone {
			t.Fatal("Tab did not skip unchanged Preview")
		}
		for row, text := range strings.Split(ansi.Strip(m.View().Content), "\n") {
			if strings.TrimSpace(text) == "Preview loop" {
				m, _ = rootUpdate(m, tea.MouseClickMsg{X: 8, Y: row, Button: tea.MouseLeft})
			}
		}
		if m.options.request != "" || m.options.id != requestID || m.options.trimFocus != trimFocusDone {
			t.Fatal("clicking unchanged Preview changed focus or requested processing")
		}
	})
	t.Run("only unprepared cut or quality changes enable Preview", func(t *testing.T) {
		for _, scenario := range []struct {
			name       string
			edit       func(*optionsState)
			canPreview bool
		}{
			{"same preview", func(*optionsState) {}, false},
			{"no prepared preview", func(options *optionsState) { options.loopReady = false }, true},
			{"changed Start", func(options *optionsState) { options.trim.StartMS = 100 }, true},
			{"changed End", func(options *optionsState) { options.trim.EndMS = 29000 }, true},
			{"changed quality", func(options *optionsState) { options.videoQuality = visual.VideoQualityHigh }, true},
			{"playback speed only", func(options *optionsState) { options.speedPercent = 50 }, false},
			{"restored cut", func(options *optionsState) { options.trim.EndMS = 29000; options.trim = options.acceptedTrim }, false},
		} {
			t.Run(scenario.name, func(t *testing.T) {
				m := newVideoTrimTestModel()
				m.browserOptionsEnabled = true
				trim := visual.VideoTrim{EndMS: 30000}
				m.options = optionsState{open: true, trimming: true, trimFocus: trimFocusPreview, sourceDurationMS: 60000, trim: trim, acceptedTrim: trim, videoQuality: visual.VideoQualityEfficient, acceptedQuality: visual.VideoQualityEfficient, loopReady: true}
				scenario.edit(&m.options)
				m, _ = rootUpdate(m, enterKey())
				if got := m.options.request == "prepareLoop"; got != scenario.canPreview {
					t.Fatalf("requested preview = %t, want %t", got, scenario.canPreview)
				}
			})
		}
	})

	t.Run("End adjustments preview the last included frame without preparing a loop", func(t *testing.T) {
		m := newVideoTrimTestModel()
		m.browserOptionsEnabled = true
		accepted := visual.VideoTrim{EndMS: 30000}
		m.options = optionsState{open: true, trimming: true, trimFocus: trimFocusEnd, id: 5, sourceDurationMS: 60000, trim: accepted, acceptedTrim: accepted, loopReady: true}
		for _, scenario := range []struct {
			key   rune
			endMS int64
		}{
			{tea.KeyLeft, 29900}, {tea.KeyRight, 30000}, {tea.KeyHome, 1000}, {tea.KeyEnd, 30000},
		} {
			m, _ = rootUpdate(m, tea.KeyPressMsg{Code: scenario.key})
			preview := m.OptionsState()
			if preview.Trim.StartMS != 0 || preview.Trim.EndMS != scenario.endMS || !preview.SourceFrame || preview.SourcePreviewMS != scenario.endMS-1 {
				t.Fatalf("preview = %+v, want the frame before End %d", preview, scenario.endMS)
			}
			if preview.Request != "" || preview.ID != 5 || m.options.acceptedTrim != accepted || !m.options.loopReady {
				t.Fatal("scrubbing prepared or replaced the accepted loop")
			}
		}
	})
	t.Run("held arrows accelerate while fresh taps direction and focus changes stay precise", func(t *testing.T) {
		m := newVideoTrimTestModel()
		m.browserOptionsEnabled = true
		m.options = optionsState{open: true, trimming: true, trimFocus: trimFocusSource, sourceDurationMS: 7200000, trim: visual.VideoTrim{EndMS: 30000}}
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyRight})
		if m.options.trim.StartMS != 100 {
			t.Fatal("single tap must move 0.1 seconds")
		}
		for repeat := 1; repeat <= 36; repeat++ {
			before := m.options.trim.StartMS
			m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyRight, IsRepeat: true})
			want := int64(100)
			if repeat >= 36 {
				want = 5000
			} else if repeat >= 20 {
				want = 1000
			}
			if got := m.options.trim.StartMS - before; got != want {
				t.Fatalf("repeat %d: step = %d ms, want %d", repeat, got, want)
			}
		}
		before := m.options.trim.StartMS
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyRight, IsRepeat: true})
		if got := m.options.trim.StartMS - before; got != 5000 {
			t.Fatalf("held step = %d ms, want 5000", got)
		}
		before = m.options.trim.StartMS
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyRight})
		if got := m.options.trim.StartMS - before; got != 100 {
			t.Fatalf("fresh tap = %d ms, want 100", got)
		}
		for range 36 {
			m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyRight, IsRepeat: true})
		}
		before = m.options.trim.StartMS
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyLeft, IsRepeat: true})
		if got := before - m.options.trim.StartMS; got != 100 {
			t.Fatalf("direction change = %d ms, want 100", got)
		}
		m.options.trimFocus = trimFocusEnd
		before = m.options.trim.EndMS
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyLeft, IsRepeat: true})
		if got := before - m.options.trim.EndMS; got != 100 {
			t.Fatalf("focus change = %d ms, want 100", got)
		}
		if m.options.request != "" {
			t.Fatal("held navigation requested processing")
		}
	})
	t.Run("source validation keeps the editor closed and reports rejection before cutting", func(t *testing.T) {
		for _, action := range []string{"sourceProtected", "sourceTracks", "sourceContainer", "sourceCodec", "sourceColor", "sourceDimensions", "sourceRate", "sourceDuration", "sourceUnsupported", "sourceSpace", "sourceTimeout", "sourceTools", "sourceBusy"} {
			t.Run(action, func(t *testing.T) {
				m := newVideoTrimTestModel()
				m.browserOptionsEnabled = true
				m.width, m.height = 80, 50
				m.options = optionsState{open: true, request: "choose", id: 5}
				m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "sourceChecking", ID: 5})
				if got := m.OptionsState().Preparation; got.Height <= 0 || got.Height > videoPreparationRows || got.Width == 0 {
					t.Fatalf("validation activity area: got %+v, want reserved rendering-wave space", got)
				}
				if m.options.trimming || m.options.request != "choose" || m.options.status != "Checking video compatibility… Esc: cancel" {
					t.Fatal("checking source did not preserve a visible cancellable selection")
				}
				m, _ = rootUpdate(m, BrowserOptionsMsg{Action: action, ID: 5})
				if m.options.trimming || m.options.request != "" || m.options.status == "" || m.options.status == "Checking video compatibility… Esc: cancel" {
					t.Fatal("rejected source did not leave selection with an error")
				}
				if !strings.Contains(ansi.Strip(m.View().Content), m.options.status) {
					t.Fatalf("rejection was not visible: %q", m.options.status)
				}
			})
		}
	})
	t.Run("preparation space and action rows remain fixed", func(t *testing.T) {
		for _, size := range [][2]int{{80, 50}, {60, 50}} {
			m := newVideoTrimTestModel()
			m.width, m.height = size[0], size[1]
			m.options = optionsState{open: true, trimming: true, sourceDurationMS: 60000, trim: visual.VideoTrim{EndMS: 30000}}
			before := strings.Split(ansi.Strip(m.View().Content), "\n")
			findRow := func(rows []string, label string) int {
				t.Helper()
				for row, content := range rows {
					if strings.Contains(content, label) {
						return row
					}
				}
				t.Fatalf("missing %q", label)
				return -1
			}
			speedRow := findRow(before, "Playback speed:")
			previewRow := findRow(before, "Preview loop")
			if previewRow-speedRow != 8 {
				t.Fatalf("%v: preview row = %d, want seven reserved rows after speed row %d", size, previewRow, speedRow)
			}
			for _, request := range []string{"prepareLoop", ""} {
				m.options.request = request
				m.options.loopReady = request == ""
				rows := strings.Split(ansi.Strip(m.View().Content), "\n")
				for _, label := range []string{"Source / Start:", "Playback speed:", "Cancel upload"} {
					if got, want := findRow(rows, label), findRow(before, label); got != want {
						t.Fatalf("%s row = %d, want %d", label, got, want)
					}
				}
				if request == "prepareLoop" {
					if got := findRow(rows, "Stop rendering"); got != previewRow {
						t.Fatalf("Stop row=%d, want Preview row %d", got, previewRow)
					}
					if got := findRow(rows, "Done — use previewed loop"); got != previewRow+1 || m.videoTrimControlEnabled(trimFocusDone) {
						t.Fatal("Done must retain its row and remain disabled during rendering")
					}
				} else if got := findRow(rows, "Preview loop"); got != previewRow {
					t.Fatal("Preview action moved after rendering")
				}
				cancelRow := findRow(rows, "Cancel upload")
				if strings.TrimSpace(rows[cancelRow+1]) != "" || strings.TrimSpace(rows[cancelRow-1]) != "" {
					t.Fatal("Cancel upload lacks surrounding blank rows")
				}
				if len(rows) > size[1] {
					t.Fatalf("%v: %d rows exceed height", size, len(rows))
				}
				statisticsRows := 3
				if size[1] == 24 {
					statisticsRows = 2
				}
				if m.options.loopReady && strings.TrimSpace(rows[findRow(rows, "Previewed:")+statisticsRows]) != "" {
					t.Fatal("missing blank row after preview statistics")
				}
			}
		}
	})
	t.Run("Done cannot receive focus or clicks until a preview is ready", func(t *testing.T) {
		m := newVideoTrimTestModel()
		m.browserOptionsEnabled = true
		m.width, m.height = 80, 50
		m.options = optionsState{open: true, trimming: true, trimFocus: trimFocusPreview, sourceDurationMS: 60000, trim: visual.VideoTrim{EndMS: 30000}}
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyTab})
		if m.options.trimFocus != trimFocusCancel {
			t.Fatal("Tab focused unavailable Done")
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
		if m.options.trimFocus != trimFocusPreview {
			t.Fatal("reverse Tab focused unavailable Done")
		}
		for row, text := range strings.Split(ansi.Strip(m.View().Content), "\n") {
			if strings.Contains(text, "Done —") {
				m, _ = rootUpdate(m, tea.MouseClickMsg{X: 8, Y: row, Button: tea.MouseLeft})
				if m.options.trimFocus != trimFocusPreview || !m.options.trimming || m.options.status != "" {
					t.Fatal("click activated unavailable Done")
				}
			}
		}
		m.options.loopReady = true
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyTab})
		if m.options.trimFocus != trimFocusDone {
			t.Fatal("ready Done was skipped")
		}
		m, _ = rootUpdate(m, enterKey())
		if m.options.trimming {
			t.Fatal("ready Done did not return to Visual")
		}
	})
	t.Run("failed preparation restores the accepted loop label and preserves drafts", func(t *testing.T) {
		for _, action := range []string{"failed", "sourceSpace", "sourceTimeout", "sourceUnsupported"} {
			t.Run(action, func(t *testing.T) {
				m := newVideoTrimTestModel()
				m.browserOptionsEnabled = true
				accepted := visual.VideoTrim{EndMS: 10000}
				draft := visual.VideoTrim{StartMS: 60000, EndMS: 90000}
				m.options = optionsState{open: true, trimming: true, request: "prepareLoop", id: 5, sourceFrame: true, loopReady: true, acceptedTrim: accepted, trim: draft}
				m, _ = rootUpdate(m, BrowserOptionsMsg{Action: action, ID: 5})
				if m.options.sourceFrame || !m.options.loopReady || m.options.acceptedTrim != accepted || m.options.trim != draft || m.options.request != "" {
					t.Fatal("failed preparation did not restore accepted-loop presentation while retaining draft")
				}
			})
		}
	})
	t.Run("start navigation seeks source and resets the proposed interval without replacing accepted bytes", func(t *testing.T) {
		for _, test := range []struct {
			name  string
			key   tea.KeyPressMsg
			start int64
		}{
			{"fine nudge", tea.KeyPressMsg{Code: tea.KeyRight}, 5050100},
			{"shift nudge", tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModShift}, 5049000},
			{"minute forward", tea.KeyPressMsg{Code: tea.KeyPgDown}, 5110000},
			{"minute backward", tea.KeyPressMsg{Code: tea.KeyPgUp}, 4990000},
			{"first scene", tea.KeyPressMsg{Code: tea.KeyHome}, 0},
			{"final scene", tea.KeyPressMsg{Code: tea.KeyEnd}, 7199000},
		} {
			t.Run(test.name, func(t *testing.T) {
				m := newVideoTrimTestModel()
				m.browserOptionsEnabled = true
				m.width, m.height = 80, 50
				accepted := visual.VideoTrim{EndMS: 30000}
				m.options = optionsState{open: true, trimming: true, trimFocus: trimFocusSource, sourceDurationMS: 7200000, trim: visual.VideoTrim{StartMS: 5050000, EndMS: 5055000}, loopReady: true, acceptedTrim: accepted}
				m, _ = rootUpdate(m, test.key)
				want := visual.VideoTrim{StartMS: test.start, EndMS: min(test.start+30000, 7200000)}
				if got := m.OptionsState().Trim; got != want {
					t.Fatalf("trim = %+v, want %+v", got, want)
				}
				if !m.options.sourceFrame || m.options.acceptedTrim != accepted || !m.options.loopReady || m.options.request != "" {
					t.Fatal("source edit changed accepted preparation or started native work")
				}
			})
		}
	})
	t.Run("end can shorten a cut without moving Start", func(t *testing.T) {
		m := newVideoTrimTestModel()
		m.browserOptionsEnabled = true
		m.options = optionsState{open: true, trimming: true, trimFocus: trimFocusEnd, sourceDurationMS: 7200000, trim: visual.VideoTrim{StartMS: 5050000, EndMS: 5080000}}
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyLeft})
		if got, want := m.options.trim, (visual.VideoTrim{StartMS: 5050000, EndMS: 5079900}); got != want {
			t.Fatalf("trim = %+v, want %+v", got, want)
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyHome})
		if m.options.trim.EndMS != 5051000 {
			t.Fatalf("minimum end = %d, want 5051000", m.options.trim.EndMS)
		}
	})
	t.Run("inline timecode entry confirms or cancels without closing the editor", func(t *testing.T) {
		m := newVideoTrimTestModel()
		m.browserOptionsEnabled = true
		m.options = optionsState{open: true, trimming: true, sourceDurationMS: 14400000, trim: visual.VideoTrim{EndMS: 30000}}
		m, _ = rootUpdate(m, enterKey())
		m.options.trimTimecode.SetValue("03:24:10.125")
		m, _ = rootUpdate(m, enterKey())
		if got, want := m.options.trim, (visual.VideoTrim{StartMS: 12250125, EndMS: 12280125}); got != want {
			t.Fatalf("trim = %+v, want %+v", got, want)
		}
		m, _ = rootUpdate(m, enterKey())
		m.options.trimTimecode.SetValue("01:99:00")
		m, _ = rootUpdate(m, enterKey())
		if !m.options.trimTimecodeEditing || !strings.Contains(m.options.status, "HH:MM:SS") {
			t.Fatal("invalid timecode was accepted")
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyEscape})
		if !m.options.trimming || m.options.request != "" || m.options.trimTimecodeEditing {
			t.Fatal("Escape cancelled the editor instead of timecode entry")
		}
	})
	t.Run("unpreviewed cut edits preserve the accepted loop used by Done and Apply", func(t *testing.T) {
		m := newVideoTrimTestModel()
		m, _ = rootUpdate(m, BrowserOptionsEnabledMsg{})
		m, _ = rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 36})
		m, _ = openVisualOptions(m)
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "loaded", ID: m.OptionsState().ID, Darkness: 70})
		m.activateOption()
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "sourceSelected", ID: m.OptionsState().ID, Kind: "video", DurationMS: 60000})
		if !m.OptionsState().Trimming || m.OptionsState().Trim.EndMS != 30000 {
			t.Fatalf("staged state = %+v", m.OptionsState())
		}
		m.options.trimFocus = trimFocusPreview
		m, _ = rootUpdate(m, enterKey())
		id := m.OptionsState().ID
		accepted := visual.VideoTrim{StartMS: 0, EndMS: 30000}
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "loopReady", VideoQuality: visual.VideoQualityEfficient, Width: 1280, Height: 720, FrameRate: visual.VideoFrameRate{Numerator: 24, Denominator: 1}, ID: id, Trim: accepted, DurationMS: 30000})
		m.options.trimFocus = trimFocusSource
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyRight})
		if m.OptionsState().Trim.StartMS != 100 || m.options.acceptedTrim != accepted || !m.options.loopReady {
			t.Fatal("draft edit lost accepted loop")
		}
		if !strings.Contains(m.View().Content, "Range: 00:00:00 - 00:00:30") {
			t.Fatal("accepted range not visible")
		}
		m.options.trimFocus = trimFocusDone
		m, _ = rootUpdate(m, enterKey())
		if m.OptionsState().Trimming {
			t.Fatal("Done did not return to Visual")
		}
		rows := strings.Split(ansi.Strip(m.View().Content), "\n")
		for rowIndex, row := range rows {
			if strings.HasPrefix(strings.TrimSpace(row), "Previewed:") {
				if rowIndex < 2 || strings.TrimSpace(rows[rowIndex-1]) != "" || strings.TrimSpace(rows[rowIndex-2]) != "Back" {
					t.Fatalf("preview details must follow Back and a blank row:\n%s", m.View().Content)
				}
			}
		}
		m.options.focus = optionsFocusApply
		m, _ = rootUpdate(m, enterKey())
		if m.OptionsState().Request != "apply" || m.options.acceptedTrim != accepted {
			t.Fatal("Apply did not retain accepted loop")
		}
	})
	t.Run("preview cells and separate controls fit normal and narrow terminals", func(t *testing.T) {
		for _, size := range [][2]int{{80, 50}, {60, 50}} {
			m := newVideoTrimTestModel()
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
			if size[1] == 50 && (!strings.Contains(rows[4], "╭") || !strings.Contains(rows[5], "│"+strings.Repeat(" ", rect.Width)+"│")) {
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

	t.Run("preparation keeps Stop and Cancel keyboard accessible while cut controls stay locked", func(t *testing.T) {
		m := newVideoTrimTestModel()
		m.browserOptionsEnabled = true
		m.width, m.height = 80, 50
		trim := visual.VideoTrim{EndMS: 30000}
		m.options = optionsState{
			open:             true,
			trimming:         true,
			trimFocus:        trimFocusPreview,
			sourceDurationMS: 60000,
			trim:             trim,
		}
		m, _ = rootUpdate(m, enterKey())
		id := m.OptionsState().ID
		if m.OptionsState().Request != "prepareLoop" {
			t.Fatal("preview did not request preparation")
		}
		if !strings.Contains(m.View().Content, "Enter: activate") || !strings.Contains(m.View().Content, "Esc: stop rendering") {
			t.Fatal("pending preparation does not explain keyboard cancellation")
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyRight})
		if m.OptionsState().Trim != trim || m.OptionsState().ID != id {
			t.Fatal("pending preparation allowed edits or a duplicate request")
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyTab})
		if m.options.trimFocus != trimFocusCancel {
			t.Fatal("Tab did not focus Cancel during preparation")
		}
		m, _ = rootUpdate(m, enterKey())
		if m.OptionsState().Request != "discardVideo" || m.OptionsState().ID == id {
			t.Fatal("Enter on Cancel did not retire preparation")
		}
		if !strings.Contains(m.View().Content, "Cancelling upload") {
			t.Fatal("cancel still displays preparation status")
		}
	})

	t.Run("Cancel remains clickable during preparation and retires its result", func(t *testing.T) {
		m := newVideoTrimTestModel()
		m.browserOptionsEnabled = true
		m.width, m.height = 80, 50
		m.options = optionsState{
			open:             true,
			id:               5,
			request:          "prepareLoop",
			video:            true,
			trimming:         true,
			sourceDurationMS: 60000,
		}
		m, _ = rootUpdate(m, tea.MouseClickMsg{X: 8, Y: 35, Button: tea.MouseLeft})
		if m.OptionsState().Request != "discardVideo" {
			t.Fatalf("cancel request = %q", m.OptionsState().Request)
		}
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "loopReady", VideoQuality: visual.VideoQualityEfficient, Width: 1280, Height: 720, FrameRate: visual.VideoFrameRate{Numerator: 24, Denominator: 1}, ID: 5, Trim: visual.VideoTrim{EndMS: 1000}, DurationMS: 1000})
		if m.options.loopReady {
			t.Fatal("cancelled result accepted")
		}
	})

	t.Run("failed replacement choice and committed warning retire new-upload controls", func(t *testing.T) {
		for _, action := range []string{"failed", "warning"} {
			m := newVideoTrimTestModel()
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
		m := newVideoTrimTestModel()
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
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "loopReady", VideoQuality: visual.VideoQualityEfficient, Width: 1280, Height: 720, FrameRate: visual.VideoFrameRate{Numerator: 24, Denominator: 1}, ID: 100, Trim: visual.VideoTrim{EndMS: 1000}, DurationMS: 1000})
		if m.options.loopReady {
			t.Fatal("stale result accepted")
		}
	})
}

func TestModel_VideoTrimCancellation(t *testing.T) {
	cancelClick := tea.MouseClickMsg{X: 8, Y: 35, Button: tea.MouseLeft}
	for _, request := range []string{"", "prepareLoop"} {
		for _, action := range []struct {
			name     string
			messages []tea.Msg
		}{
			{"Escape", []tea.Msg{tea.KeyPressMsg{Code: tea.KeyEscape}}},
			{"Q", []tea.Msg{tea.KeyPressMsg{Code: 'q'}}},
			{"Cancel click", []tea.Msg{cancelClick}},
			{"Tab then Enter", []tea.Msg{tea.KeyPressMsg{Code: tea.KeyTab}, enterKey()}},
		} {
			if request == "prepareLoop" && (action.name == "Escape" || action.name == "Q") {
				continue // Rendering Escape stops only the render; explicit Cancel still discards.
			}
			name := "editing"
			if request != "" {
				name = request
			}
			t.Run(name+" cancels once with "+action.name, func(t *testing.T) {
				m := newVideoTrimTestModel()
				m.browserOptionsEnabled = true
				m.width, m.height = 80, 50
				m.options = optionsState{
					open:             true,
					id:               5,
					request:          request,
					darkness:         70,
					video:            true,
					trimming:         true,
					newVideo:         true,
					trimFocus:        trimFocusDone,
					sourceDurationMS: 60000,
					trim:             visual.VideoTrim{EndMS: 30000},
				}
				for _, message := range action.messages {
					m, _ = rootUpdate(m, message)
				}
				if got := m.OptionsState(); got.Request != "discardVideo" || got.ID != 6 {
					t.Fatalf("cancel state = %+v, want discardVideo with ID 6", got)
				}
				for _, message := range []tea.Msg{
					tea.KeyPressMsg{Code: tea.KeyEscape}, tea.KeyPressMsg{Code: tea.KeyEscape},
					tea.KeyPressMsg{Code: 'q'}, tea.KeyPressMsg{Code: 'q'},
					enterKey(), tea.KeyPressMsg{Code: tea.KeySpace}, cancelClick, cancelClick,
				} {
					m, _ = rootUpdate(m, message)
					if got := m.OptionsState(); got.Request != "discardVideo" || got.ID != 6 {
						t.Fatalf("repeated %T changed discard ownership: got %+v, want discardVideo with ID 6", message, got)
					}
				}
				latePreview := BrowserOptionsMsg{
					Action:       "loopReady",
					ID:           5,
					VideoQuality: visual.VideoQualityEfficient,
					Width:        1280,
					Height:       720,
					FrameRate:    visual.VideoFrameRate{Numerator: 24, Denominator: 1},
					Trim:         visual.VideoTrim{EndMS: 1000},
					DurationMS:   1000,
				}
				m, _ = rootUpdate(m, latePreview)
				if m.options.loopReady || m.OptionsState().ID != 6 || m.OptionsState().Request != "discardVideo" {
					t.Fatalf("late preview changed cancellation: %+v", m.OptionsState())
				}
				m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "discarded", ID: 6, Background: true, Kind: "image"})
				if !m.options.open || m.options.trimming || m.options.newVideo || m.options.loopReady || !m.options.hasBackground || m.options.video || m.options.request != "" {
					t.Fatalf("discard completion did not restore saved-image Visual: %+v", m.OptionsState())
				}
				if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Background darkness: 70%") || strings.Contains(view, "Cancel upload") {
					t.Fatalf("discard completion shows the wrong screen:\n%s", view)
				}
				m, _ = rootUpdate(m, latePreview)
				if m.options.loopReady || m.options.trimming {
					t.Fatal("late preview restored the cancelled editor")
				}
				focus := m.options.focus
				m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyTab})
				if m.options.focus == focus || m.options.request != "" {
					t.Fatal("Visual controls are not usable after cancellation")
				}
			})
		}
	}

	t.Run("cut and quality controls remain locked during preparation and cancellation", func(t *testing.T) {
		for _, request := range []string{"prepareLoop", "stopPreview", "discardVideo"} {
			for _, control := range []struct {
				name  string
				focus int
			}{
				{"Source / Start", trimFocusSource},
				{"End", trimFocusEnd},
				{"Video quality", trimFocusQuality},
			} {
				t.Run(request+" locks "+control.name, func(t *testing.T) {
					m := newVideoTrimTestModel()
					m.browserOptionsEnabled = true
					m.width, m.height = 80, 50
					trim := visual.VideoTrim{StartMS: 1000, EndMS: 30000}
					m.options = optionsState{
						open:             true,
						id:               5,
						request:          request,
						videoQuality:     visual.VideoQualityEfficient,
						trimming:         true,
						trimFocus:        control.focus,
						sourceDurationMS: 60000,

						trim: trim,
					}
					row := -1
					for index, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
						if strings.HasPrefix(strings.TrimSpace(line), control.name+":") {
							row = index
						}
					}
					if row < 0 {
						t.Fatal("missing locked control")
					}
					for _, message := range []tea.Msg{
						tea.KeyPressMsg{Code: tea.KeyRight}, tea.KeyPressMsg{Code: tea.KeyHome}, enterKey(),
						tea.MouseClickMsg{X: 8, Y: row, Button: tea.MouseLeft},
					} {
						m, _ = rootUpdate(m, message)
						if got := m.OptionsState(); got.ID != 5 || got.Request != request || got.Trim != trim || got.VideoQuality != visual.VideoQualityEfficient {
							t.Fatalf("pending %s allowed %s changes: %+v", request, control.name, got)
						}
					}
				})
			}
		}
	})
}

func TestModel_StopVideoRendering(t *testing.T) {
	t.Run("explicit Cancel remains available while rendering joins", func(t *testing.T) {
		m := newVideoTrimTestModel()
		m.browserOptionsEnabled = true
		m.options = optionsState{open: true, id: 6, request: "stopPreview", trimming: true, trimFocus: trimFocusPreview}
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyTab})
		m, _ = rootUpdate(m, enterKey())
		if got := m.OptionsState(); got.ID != 7 || got.Request != "discardVideo" {
			t.Fatalf("explicit Cancel unavailable: %+v", got)
		}
	})
	t.Run("Escape stops rendering and retains source cut and accepted loop", func(t *testing.T) {
		m := newVideoTrimTestModel()
		m.browserOptionsEnabled = true
		m.width, m.height = 80, 50
		trim := visual.VideoTrim{StartMS: 1000, EndMS: 5000}
		m.options = optionsState{open: true, id: 5, request: "prepareLoop", trimming: true, newVideo: true, loopReady: true, trim: trim, acceptedTrim: trim, sourceDurationMS: 60000, trimFocus: trimFocusPreview}
		view := strings.Split(ansi.Strip(m.View().Content), "\n")
		stopRow, cancelRow := -1, -1
		for row, line := range view {
			switch strings.TrimSpace(line) {
			case "Stop rendering":
				stopRow = row
			case "Cancel upload":
				cancelRow = row
			}
		}
		if stopRow < 0 || cancelRow <= stopRow || strings.TrimSpace(view[cancelRow-1]) != "" {
			t.Fatalf("render actions lack separation:\n%s", strings.Join(view, "\n"))
		}
		m, _ = rootUpdate(m, escapeKey())
		if got := m.OptionsState(); got.Request != "stopPreview" || got.ID != 6 || !got.Trimming || got.Trim != trim || !m.options.loopReady {
			t.Fatalf("stop discarded editor: %+v", got)
		}
		m, _ = rootUpdate(m, escapeKey())
		if m.OptionsState().ID != 6 {
			t.Fatal("repeated Stop changed operation ownership")
		}
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "previewStopped", ID: 6})
		if got := m.OptionsState(); got.Request != "" || !got.Trimming || got.Trim != trim || !m.options.loopReady {
			t.Fatalf("stop completion lost editor: %+v", got)
		}
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "loopReady", ID: 5})
		if m.OptionsState().ID != 6 || m.options.acceptedTrim != trim {
			t.Fatal("retired rendering result changed the retained cut")
		}
	})
}

func TestModel_AcceptedVideoRangeLabel(t *testing.T) {
	for _, scenario := range []struct {
		description  string
		startMS      int64
		endMS        int64
		speedPercent int
		want         string
	}{
		{"short loop", 0, 30000, 100, "Range: 00:00:00 - 00:00:30 · plays 30.00s"},
		{"crosses minute", 57000, 60000, 100, "Range: 00:00:57 - 00:01:00 · plays 3.00s"},
		{"long source", 5683000, 5699000, 100, "Range: 01:34:43 - 01:34:59 · plays 16.00s"},
		{"fractional cut", 10250, 11250, 100, "Range: 00:00:10.250 - 00:00:11.250 · plays 1.00s"},
		{"slower playback", 10000, 26000, 50, "Range: 00:00:10 - 00:00:26 · plays 32.00s"},
	} {
		t.Run(scenario.description, func(t *testing.T) {
			model := newVideoTrimTestModel()
			model.options.acceptedTrim = visual.VideoTrim{StartMS: scenario.startMS, EndMS: scenario.endMS}
			model.options.loopDurationMS = scenario.endMS - scenario.startMS
			model.options.speedPercent = scenario.speedPercent
			if got := model.acceptedVideoRangeLabel(); got != scenario.want {
				t.Fatalf("range label = %q, want %q", got, scenario.want)
			}
		})
	}
}
