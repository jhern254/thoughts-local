package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/visual"
)

func TestModel_VideoQuality(t *testing.T) {
	t.Run("unpreviewed quality changes preserve accepted geometry and quality", func(t *testing.T) {
		m := newRootTestModel()
		m.browserOptionsEnabled = true
		m.width, m.height = 80, 36
		m.options = optionsState{
			open:             true,
			video:            true,
			newVideo:         true,
			trimming:         true,
			videoQuality:     visual.VideoQualityEfficient,
			trimFocus:        trimFocusQuality,
			sourceDurationMS: 60000,
			trim:             visual.VideoTrim{EndMS: 2000},
			request:          "prepareLoop",
			id:               1,
		}
		m, _ = rootUpdate(m, BrowserOptionsMsg{
			Action:       "loopReady",
			ID:           1,
			Trim:         visual.VideoTrim{EndMS: 2000},
			DurationMS:   2000,
			VideoQuality: visual.VideoQualityEfficient,
			Width:        1280,
			Height:       720,
			FrameRate:    visual.VideoFrameRate{Numerator: 30, Denominator: 1},
		})
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyEnd})
		if m.OptionsState().VideoQuality != visual.VideoQualityMaximum || !m.options.loopReady || m.options.acceptedQuality != visual.VideoQualityEfficient {
			t.Fatal("quality edit replaced the acknowledged preview")
		}
		view := m.View().Content
		for _, label := range []string{"Video quality: Maximum", "Efficient · 1280×720 · 30.00 FPS", "Preview loop to use Maximum"} {
			if !strings.Contains(view, label) {
				t.Fatalf("missing %q in %s", label, view)
			}
		}
		m.options.trimFocus = trimFocusDone
		m, _ = rootUpdate(m, enterKey())
		m.options.focus = optionsFocusApply
		m, _ = rootUpdate(m, enterKey())
		if m.OptionsState().Request != "apply" || m.options.acceptedQuality != visual.VideoQualityEfficient {
			t.Fatal("Apply lost accepted quality")
		}
	})
	t.Run("quality is available before choosing a video", func(t *testing.T) {
		m := newRootTestModel()
		m, _ = rootUpdate(m, BrowserOptionsEnabledMsg{})
		m, _ = openVisualOptions(m)
		if m.OptionsState().VideoQuality != visual.VideoQualityEfficient {
			t.Fatal("initial preference is not Efficient")
		}
		m.options.request = ""
		m.options.focus = optionsFocusQuality
		m, _ = rootUpdate(m, tea.KeyPressMsg{Code: tea.KeyRight})
		if m.OptionsState().VideoQuality != visual.VideoQualityHigh || m.OptionsState().Request != "" {
			t.Fatal("preference selection triggered processing")
		}
	})
}
