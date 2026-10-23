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
				want := action.want
				if focus == trimFocusEnd && want == 60000 {
					want = 31251
				}
				if got := position(); got != want {
					t.Fatalf("focus %d key %d: got %d ms, want %d", focus, action.code, got, want)
				}
			}
			if !strings.Contains(m.View().Content, "←/→: 0.1s · Shift: 1s") {
				t.Fatal("help does not describe the arrow increments")
			}
		}
	})
	t.Run("end stays within thirty seconds of start and the source boundary", func(t *testing.T) {
		for _, tc := range []struct {
			name       string
			focus      int
			trim       visual.VideoTrim
			positionMS int64
			key        tea.KeyPressMsg
			want       visual.VideoTrim
			wantMax    string
		}{
			{"right at maximum", trimFocusEnd, visual.VideoTrim{StartMS: 1251, EndMS: 31251}, 0, tea.KeyPressMsg{Code: tea.KeyRight}, visual.VideoTrim{StartMS: 1251, EndMS: 31251}, "31.25"},
			{"shift right crossing maximum", trimFocusEnd, visual.VideoTrim{StartMS: 1251, EndMS: 31000}, 0, tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift}, visual.VideoTrim{StartMS: 1251, EndMS: 31251}, "31.25"},
			{"end key uses relative maximum", trimFocusEnd, visual.VideoTrim{StartMS: 1251, EndMS: 5000}, 0, tea.KeyPressMsg{Code: tea.KeyEnd}, visual.VideoTrim{StartMS: 1251, EndMS: 31251}, "31.25"},
			{"source position cannot bypass maximum", trimFocusEnd, visual.VideoTrim{StartMS: 1251, EndMS: 5000}, 60000, enterKey(), visual.VideoTrim{StartMS: 1251, EndMS: 31251}, "31.25"},
			{"late cut ends at source boundary", trimFocusEnd, visual.VideoTrim{StartMS: 57000, EndMS: 58000}, 60000, tea.KeyPressMsg{Code: tea.KeyEnd}, visual.VideoTrim{StartMS: 57000, EndMS: 60000}, "60.00"},
			{"moving start earlier bounds end", trimFocusStart, visual.VideoTrim{StartMS: 10000, EndMS: 40000}, 0, tea.KeyPressMsg{Code: tea.KeyLeft}, visual.VideoTrim{StartMS: 9900, EndMS: 39900}, "39.90"},
			{"start home bounds end", trimFocusStart, visual.VideoTrim{StartMS: 10000, EndMS: 40000}, 0, tea.KeyPressMsg{Code: tea.KeyHome}, visual.VideoTrim{EndMS: 30000}, "30.00"},
			{"start at source position bounds end", trimFocusStart, visual.VideoTrim{StartMS: 10000, EndMS: 40000}, 1251, enterKey(), visual.VideoTrim{StartMS: 1251, EndMS: 31251}, "31.25"},
			{"source cursor still reaches full source", trimFocusSource, visual.VideoTrim{StartMS: 1251, EndMS: 31251}, 0, tea.KeyPressMsg{Code: tea.KeyEnd}, visual.VideoTrim{StartMS: 1251, EndMS: 31251}, "31.25"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				m := newRootTestModel()
				m.browserOptionsEnabled = true
				m.width, m.height = 80, 36
				accepted := visual.VideoTrim{StartMS: 10000, EndMS: 40000}
				m.options = optionsState{
					open:             true,
					trimming:         true,
					trimFocus:        tc.focus,
					sourceDurationMS: 60000,
					sourcePositionMS: tc.positionMS,
					trim:             tc.trim,
					loopReady:        true,
					acceptedTrim:     accepted,
				}
				m, _ = rootUpdate(m, tc.key)
				if got := m.OptionsState().Trim; got != tc.want {
					t.Fatalf("trim = %+v, want %+v", got, tc.want)
				}
				if got := m.options.acceptedTrim; got != accepted || !m.options.loopReady || m.OptionsState().Request != "" {
					t.Fatalf("draft edit changed accepted loop or requested preparation: %+v", m.OptionsState())
				}
				if tc.focus == trimFocusSource && m.options.sourcePositionMS != 60000 {
					t.Fatalf("source cursor = %d, want 60000", m.options.sourcePositionMS)
				}
				if !strings.Contains(ansi.Strip(m.View().Content), "End: "+trimSeconds(tc.want.EndMS)+"s (max "+tc.wantMax+"s)") {
					t.Fatalf("End control does not show current maximum:\n%s", m.View().Content)
				}
			})
		}
	})
	t.Run("unpreviewed cut edits preserve the accepted loop used by Done and Apply", func(t *testing.T) {
		m := newRootTestModel()
		m, _ = rootUpdate(m, BrowserOptionsEnabledMsg{})
		m, _ = rootUpdate(m, tea.WindowSizeMsg{Width: 80, Height: 36})
		m, _ = openVisualOptions(m)
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
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "loopReady", VideoQuality: visual.VideoQualityEfficient, Width: 1280, Height: 720, FrameRate: visual.VideoFrameRate{Numerator: 24, Denominator: 1}, ID: id, Trim: accepted, DurationMS: 30000})
		m.options.trimFocus = trimFocusStart
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyRight})
		if m.OptionsState().Trim.StartMS != 100 || m.options.acceptedTrim != accepted || !m.options.loopReady {
			t.Fatal("draft edit lost accepted loop")
		}
		if !strings.Contains(m.View().Content, "Range: 0.00–30.00s") {
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
			if size[1] == 36 && (!strings.Contains(rows[4], "╭") || !strings.Contains(rows[5], "│"+strings.Repeat(" ", rect.Width)+"│")) {
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

	t.Run("preparation keeps Cancel keyboard accessible while cut controls stay locked", func(t *testing.T) {
		m := newRootTestModel()
		m.browserOptionsEnabled = true
		m.width, m.height = 80, 36
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
		if !strings.Contains(m.View().Content, "Tab: Cancel") {
			t.Fatal("pending preparation does not explain keyboard cancellation")
		}
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyRight})
		m, _ = rootUpdate(m, enterKey())
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
		m, _ = rootUpdate(m, tea.MouseClickMsg{X: 8, Y: 27, Button: tea.MouseLeft})
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
		m, _ = rootUpdate(m, BrowserOptionsMsg{Action: "loopReady", VideoQuality: visual.VideoQualityEfficient, Width: 1280, Height: 720, FrameRate: visual.VideoFrameRate{Numerator: 24, Denominator: 1}, ID: 100, Trim: visual.VideoTrim{EndMS: 1000}, DurationMS: 1000})
		if m.options.loopReady {
			t.Fatal("stale result accepted")
		}
	})
}

func TestModel_VideoTrimCancellation(t *testing.T) {
	cancelClick := tea.MouseClickMsg{X: 8, Y: 27, Button: tea.MouseLeft}
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
			name := "editing"
			if request != "" {
				name = request
			}
			t.Run(name+" cancels once with "+action.name, func(t *testing.T) {
				m := newRootTestModel()
				m.browserOptionsEnabled = true
				m.width, m.height = 80, 36
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
		for _, request := range []string{"prepareLoop", "discardVideo"} {
			for _, control := range []struct {
				name  string
				focus int
				row   int
			}{
				{"Start", trimFocusStart, 21},
				{"End", trimFocusEnd, 22},
				{"Quality", trimFocusQuality, 23},
			} {
				t.Run(request+" locks "+control.name, func(t *testing.T) {
					m := newRootTestModel()
					m.browserOptionsEnabled = true
					m.width, m.height = 80, 36
					trim := visual.VideoTrim{StartMS: 1000, EndMS: 30000}
					m.options = optionsState{
						open:             true,
						id:               5,
						request:          request,
						videoQuality:     visual.VideoQualityEfficient,
						trimming:         true,
						trimFocus:        control.focus,
						sourceDurationMS: 60000,
						sourcePositionMS: 9000,
						trim:             trim,
					}
					for _, message := range []tea.Msg{
						tea.KeyPressMsg{Code: tea.KeyRight}, tea.KeyPressMsg{Code: tea.KeyHome}, enterKey(),
						tea.MouseClickMsg{X: 8, Y: control.row, Button: tea.MouseLeft},
					} {
						m, _ = rootUpdate(m, message)
						if got := m.OptionsState(); got.ID != 5 || got.Request != request || got.Trim != trim || got.SourcePositionMS != 9000 || got.VideoQuality != visual.VideoQualityEfficient {
							t.Fatalf("pending %s allowed %s changes: %+v", request, control.name, got)
						}
					}
				})
			}
		}
	})
}
