package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/visual"
)

type BrowserOptionsEnabledMsg struct{}

// BrowserOptionsMsg carries only bounded visual controls. Media bytes use
// validated HTTP routes; no frames or authored content travel in these messages.
type BrowserOptionsMsg struct {
	VideoQuality    visual.VideoQuality    `json:"videoQuality"`
	FrameRate       visual.VideoFrameRate  `json:"frameRate"`
	Width           int                    `json:"width"`
	Height          int                    `json:"height"`
	Action          string                 `json:"action"`
	ID              uint64                 `json:"id"`
	DurationMS      int64                  `json:"durationMS"` // Full source length for source replies; prepared loop length for loopReady.
	Trim            visual.VideoTrim       `json:"trim"`       // Source interval used by the acknowledged loopReady preview.
	Darkness        int                    `json:"darkness"`
	Background      bool                   `json:"background"`
	Kind            string                 `json:"kind"`
	SpeedPercent    int                    `json:"speedPercent"`
	UserPaused      bool                   `json:"userPaused"`
	PlaybackBlocked bool                   `json:"playbackBlocked"` // Playback failed or reduced-motion settings prevent play; separate from user Pause.
	Framing         data.BackgroundFraming `json:"framing"`
}

// PreviewRect reserves space for browser media in terminal cells, not pixels.
type PreviewRect struct{ X, Y, Width, Height int }

// BrowserOptionsState tells the browser which visual controls and preview to show.
// ID changes for each browser operation so late results cannot affect a new page.
type BrowserOptionsState struct {
	MaxVideoSourceBytes      int64                  `json:"maxVideoSourceBytes"`
	MaxVideoSourceDurationMS int64                  `json:"maxVideoSourceDurationMS"`
	VideoQuality             visual.VideoQuality    `json:"videoQuality"`
	Open                     bool                   `json:"open"`
	ID                       uint64                 `json:"id"`
	Request                  string                 `json:"request"`
	Editing                  bool                   `json:"editing"` // Framing controls (zoom/position) are open.
	Framing                  data.BackgroundFraming `json:"framing"`
	Darkness                 int                    `json:"darkness"`
	SpeedPercent             int                    `json:"speedPercent"`
	UserPaused               bool                   `json:"userPaused"`
	Preview                  PreviewRect            `json:"preview"`
	Preparation              PreviewRect            `json:"preparation"`     // Reserved cells keep controls fixed while the browser shows loading activity.
	Trimming                 bool                   `json:"trimming"`        // Cut-selection controls are open; this does not mean encoding is running.
	SourceFrame              bool                   `json:"sourceFrame"`     // Show the original-source frame instead of the prepared loop.
	SourcePreviewMS          int64                  `json:"sourcePreviewMS"` // Source-relative position of the boundary being adjusted.
	Trim                     visual.VideoTrim       `json:"trim"`            // Draft start/end times in source milliseconds; may differ from the accepted preview.
}

const (
	optionsFocusChooseBackground = iota
	optionsFocusFit
	optionsFocusAdjust
	optionsFocusDarkness
	optionsFocusQuality
	optionsFocusSpeed
	optionsFocusPlayback
	optionsFocusPreview
	optionsFocusTrim
	optionsFocusApply
	optionsFocusRemoveBackground
	optionsFocusBack
	optionsFocusCount
)

type optionsState struct {
	open              bool
	menu              bool
	menuFocus         int
	id                uint64
	request           string
	darkness          int
	focus             int
	status            string
	hasBackground     bool
	video             bool
	videoQuality      visual.VideoQuality
	speedPercent      int
	userPaused        bool
	playbackBlocked   bool
	framing           data.BackgroundFraming
	editing           bool
	framingFocus      int
	framingBeforeEdit data.BackgroundFraming

	// Cut controls are drafts. Only loopReady/acceptedTrim identify the loop
	// acknowledged by the browser; moving draft markers must never revoke it.
	newVideo            bool // An unsaved source is selected; Edit loop remains available until Apply/discard.
	trimming            bool
	trimFocus           int
	trimTimecode        textinput.Model
	trimTimecodeEditing bool // Keyboard input is editing the Source / Start timecode.
	trimRepeatKey       string
	trimRepeatFocus     int
	trimRepeatCount     int
	sourcePreviewMS     int64
	sourceFrame         bool
	sourceDurationMS    int64
	trim                visual.VideoTrim
	acceptedQuality     visual.VideoQuality
	acceptedWidth       int
	acceptedHeight      int
	acceptedRate        visual.VideoFrameRate
	acceptedTrim        visual.VideoTrim // Source cuts for the acknowledged loop, retained while draft cuts change.
	loopDurationMS      int64
	loopReady           bool
}

func (m *Model) requestOptions(action string) {
	m.options.id++
	m.options.request = action
}

func (m Model) OptionsState() BrowserOptionsState {
	if m.options.menu || m.editorPreferences.open {
		return BrowserOptionsState{}
	}
	_, rect := m.optionsLayout()
	return BrowserOptionsState{
		MaxVideoSourceBytes:      visual.MaxVideoSourceBytes,
		MaxVideoSourceDurationMS: visual.MaxVideoSourceDuration * 1000,
		Open:                     m.options.open,
		VideoQuality:             m.options.videoQuality,
		ID:                       m.options.id,
		Request:                  m.options.request,
		Editing:                  m.options.editing,
		Framing:                  m.options.framing,
		Darkness:                 m.options.darkness,
		SpeedPercent:             m.options.speedPercent,
		UserPaused:               m.options.userPaused,
		Preview:                  rect,
		Preparation:              m.videoPreparationRect(),
		Trimming:                 m.options.trimming,
		SourceFrame:              m.options.sourceFrame,
		SourcePreviewMS:          m.options.sourcePreviewMS,
		Trim:                     m.options.trim,
	}
}

func (m Model) updateBrowserOptions(msg BrowserOptionsMsg) (tea.Model, tea.Cmd) {
	if !m.browserOptionsEnabled || m.exitPromptOpen {
		return m, nil
	}
	if msg.Action == "open" {
		if m.speechSetup.open {
			return m, nil
		}
		if !m.options.open && (m.speechSetup.service != nil || m.editorPreferences.service != nil) {
			m.options.open = true
			m.options.menu = true
			m.options.request = ""
			return m, nil
		}
		if !m.options.open {
			m.options.open = true
			m.options.trimming = false
			m.options.newVideo = false
			m.options.loopReady = false
			m.options.editing = false
			m.options.hasBackground = false
			m.options.framing = data.DefaultBackgroundFraming()
			m.options.focus = optionsFocusChooseBackground
			m.options.darkness = data.DefaultBackgroundDarkness
			m.options.speedPercent = 100
			m.options.videoQuality = visual.VideoQualityEfficient
			m.options.userPaused = false
			m.options.status = "Loading visual…"
			m.requestOptions("load")
		}
		return m, nil
	}
	if msg.Action == "playback" {
		if m.options.open && msg.ID == m.options.id {
			m.options.playbackBlocked = msg.PlaybackBlocked
		}
		return m, nil
	}
	if msg.Action == "reframe" {
		if m.options.open && m.options.editing && msg.ID == m.options.id && msg.Framing.Valid() && msg.Framing.Fit == "fill" {
			m.options.framing = msg.Framing
			m.options.framingFocus = framingFocusPreview
		}
		return m, nil
	}
	if !m.options.open || msg.ID != m.options.id || m.options.request == "" {
		return m, nil
	}
	switch msg.Action {
	case "sourceSelected":
		if m.options.request != "choose" || msg.DurationMS < 1000 || msg.DurationMS > visual.MaxVideoSourceDuration*1000 {
			return m, nil
		}
		m.options.video = true
		m.options.newVideo = true
		fit := m.options.framing.Fit
		m.options.framing = data.DefaultBackgroundFraming()
		m.options.framing.Fit = fit
		m.options.loopReady = false
		m.options.hasBackground = true
		m.options.trimming = true
		m.options.sourceFrame = !m.options.loopReady
		m.options.sourcePreviewMS = 0
		m.options.trimRepeatCount = 0
		m.options.trimFocus = trimFocusSource
		m.options.trimTimecodeEditing = false
		m.options.sourceDurationMS = msg.DurationMS
		m.options.trim = visual.VideoTrim{StartMS: 0, EndMS: min(visual.MaxVideoDuration*1000, msg.DurationMS)}
		m.options.status = "Set cut points, then Preview loop."
	case "sourceChecking":
		if m.options.request != "choose" {
			return m, nil
		}
		m.options.status = "Checking video compatibility… Esc: cancel"
		return m, nil
	case "sourceBounds":
		if m.options.request != "prepareLoop" {
			return m, nil
		}
		m.options.status = "Verified source bounds differ. Adjust Start/End and preview again."
	case "loopReady":
		if m.options.request != "prepareLoop" || !msg.Trim.Valid(m.options.sourceDurationMS) || msg.DurationMS < 1000 || msg.DurationMS > 30000 {
			return m, nil
		}
		if !msg.VideoQuality.Valid() || !msg.FrameRate.Valid() || msg.Width < 2 || msg.Width > 1920 || msg.Height < 2 || msg.Height > 1920 || msg.Width*msg.Height > 1920*1080 {
			return m, nil
		}
		m.options.acceptedQuality = msg.VideoQuality
		m.options.acceptedWidth, m.options.acceptedHeight = msg.Width, msg.Height
		m.options.acceptedRate = msg.FrameRate
		m.options.acceptedTrim = msg.Trim
		m.options.loopDurationMS = msg.DurationMS
		m.options.loopReady = true
		m.options.sourceFrame = false
		m.options.status = "Preview ready. Done uses this loop."
		if m.options.trimFocus == trimFocusPreview {
			m.options.trimFocus = trimFocusDone
		}
	case "discarded":
		if m.options.request != "discardVideo" {
			return m, nil
		}
		m.options.trimTimecodeEditing = false
		m.options.trimming = false
		m.options.newVideo = false
		m.options.loopReady = false
		m.options.hasBackground = msg.Background
		m.options.video = msg.Kind == "video"
		m.options.status = ""
	case "loaded":
		if m.options.request != "load" {
			return m, nil
		}
		if msg.VideoQuality.Valid() {
			m.options.videoQuality = msg.VideoQuality
		}
		m.options.darkness = max(0, min(95, msg.Darkness))
		m.options.hasBackground = msg.Background
		m.options.video = msg.Kind == "video"
		if msg.SpeedPercent >= 25 && msg.SpeedPercent <= 100 {
			m.options.speedPercent = msg.SpeedPercent
		}
		m.options.userPaused = msg.UserPaused
		m.options.playbackBlocked = msg.PlaybackBlocked
		if msg.Framing.Valid() {
			m.options.framing = msg.Framing
		}
		m.options.status = "JPEG/PNG · MP4/MOV SDR H.264 · source ≤4h / 8 GiB · loop 1–30s"
	case "selected":
		if m.options.request != "choose" {
			return m, nil
		}
		m.options.hasBackground = true
		m.options.video = msg.Kind == "video"
		m.options.newVideo = false
		fit := m.options.framing.Fit
		m.options.framing = data.DefaultBackgroundFraming()
		m.options.framing.Fit = fit
		m.options.status = "Preview only. Apply to save."
	case "played", "previewed":
		if m.options.request != "play" && m.options.request != "preview" {
			return m, nil
		}
		m.options.status = ""
	case "cancelled":
		if m.options.request != "choose" {
			return m, nil
		}
		m.options.status = ""
	case "saved", "warning":
		if m.options.request != "apply" && m.options.request != "remove" {
			return m, nil
		}
		m.options.newVideo = false
		m.options.loopReady = false
		m.options.trimming = false
		if msg.VideoQuality.Valid() {
			m.options.videoQuality = msg.VideoQuality
		}
		m.options.darkness = max(0, min(95, msg.Darkness))
		m.options.hasBackground = msg.Background
		m.options.video = msg.Kind == "video"
		if msg.SpeedPercent >= 25 && msg.SpeedPercent <= 100 {
			m.options.speedPercent = msg.SpeedPercent
		}
		m.options.userPaused = msg.UserPaused
		m.options.playbackBlocked = msg.PlaybackBlocked
		if msg.Framing.Valid() {
			m.options.framing = msg.Framing
		}
		if msg.Action == "saved" {
			m.options.open = false
		} else {
			m.options.status = "Saved. Some background files could not be removed."
		}
	case "failed", "sourceSpace", "sourceTimeout", "sourceUnsupported", "sourceCodec", "sourceColor", "sourceDimensions", "sourceRate", "sourceDuration", "sourceSize", "sourceTools", "sourceBusy":
		if m.options.request == "prepareLoop" && m.options.loopReady {
			m.options.sourceFrame = false
		}
		if m.options.request == "choose" {
			m.options.newVideo = false
			m.options.loopReady = false
			m.options.trimming = false
			m.options.hasBackground = msg.Background
			m.options.video = msg.Kind == "video"
			if msg.SpeedPercent >= 25 && msg.SpeedPercent <= 100 {
				m.options.speedPercent = msg.SpeedPercent
			}
			m.options.userPaused = msg.UserPaused
			m.options.playbackBlocked = msg.PlaybackBlocked
			if msg.Framing.Valid() {
				m.options.framing = msg.Framing
			}
		}
		m.options.status = "Could not load or apply. Check media limits and retry."
		if m.options.request == "prepareLoop" {
			m.options.status = "Could not prepare this loop. Retry or choose another cut."
		}
		switch msg.Action {
		case "sourceCodec":
			m.options.status = "Unsupported video codec. H.264 is required."
		case "sourceColor":
			m.options.status = "Unsupported video color. Use 8-bit SDR Rec.709."
		case "sourceDimensions":
			m.options.status = "Video exceeds 4096 pixels per side or 9 million pixels."
		case "sourceRate":
			m.options.status = "Unsupported frame rate. Videos must be at most 60 FPS."
		case "sourceDuration":
			m.options.status = "Unsupported duration. Videos must be 1 second–4 hours."
		case "sourceSize":
			m.options.status = "Video exceeds the 8 GiB upload limit."
		case "sourceTools":
			m.options.status = "Video processing unavailable. Install FFmpeg and ffprobe."
		case "sourceBusy":
			m.options.status = "Another visual operation is running. Try again shortly."
		case "sourceSpace":
			m.options.status = "Not enough temporary disk space for the video source."
		case "sourceTimeout":
			m.options.status = "Video operation timed out. Try again or use a shorter cut."
		case "sourceUnsupported":
			m.options.status = "Could not validate this video. Use an intact H.264 MP4/MOV."
		}

	default:
		return m, nil
	}
	m.options.request = ""
	return m, nil
}

func (m Model) updateOptionsInput(message tea.Msg) (tea.Model, tea.Cmd) {
	if m.editorPreferences.open {
		return m.updateEditorPreferences(message)
	}
	if m.options.menu {
		return m.updateOptionsMenu(message)
	}
	if m.options.trimming {
		return m.updateVideoTrimInput(message)
	}
	if m.options.editing {
		return m.updateFramingInput(message)
	}
	busy := m.options.request != ""
	switch msg := message.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc":
			if (m.options.request != "apply" && m.options.request != "remove") || (m.options.request == "apply" && m.options.video) {
				m.options.open = false
				if m.speechSetup.service != nil {
					m.options.open = true
					m.options.menu = true
				}
				m.requestOptions("")
			}
		case "tab", "down":
			if !busy {
				m.options.focus = m.nextOptionsFocus(1)
			}
		case "shift+tab", "up":
			if !busy {
				m.options.focus = m.nextOptionsFocus(-1)
			}
		case "left", "right", "home", "end":
			if !busy && m.options.focus == optionsFocusQuality {
				m.setVideoQuality(msg.String())
			}
			if !busy && m.options.focus == optionsFocusSpeed && m.options.video {
				switch msg.String() {
				case "left":
					m.options.speedPercent = max(25, m.options.speedPercent-5)
				case "right":
					m.options.speedPercent = min(100, m.options.speedPercent+5)
				case "home":
					m.options.speedPercent = 25
				case "end":
					m.options.speedPercent = 100
				}
			}
			if !busy && m.options.focus == optionsFocusFit && (msg.String() == "left" || msg.String() == "right") {
				m.toggleBackgroundFit()
			}
			if !busy && m.options.focus == optionsFocusDarkness {
				switch msg.String() {
				case "left":
					m.options.darkness = max(0, m.options.darkness-1)
				case "right":
					m.options.darkness = min(95, m.options.darkness+1)
				case "home":
					m.options.darkness = 0
				case "end":
					m.options.darkness = 95
				}
			}
		case "enter", "space":
			if !busy {
				m.activateOption()
			}
		}
	case tea.MouseClickMsg:
		if !busy && msg.Button == tea.MouseLeft {
			m.clickOption(msg.X, msg.Y, true)
		}
	case tea.MouseMotionMsg:
		if !busy && msg.Button == tea.MouseLeft {
			m.clickOption(msg.X, msg.Y, false)
		}
	}
	return m, nil
}

func (m *Model) activateOption() {
	switch m.options.focus {
	case optionsFocusChooseBackground:
		m.options.status = "Choose a JPEG/PNG image or SDR H.264 MP4/MOV…"
		m.requestOptions("choose")
	case optionsFocusQuality:
		m.cycleVideoQuality()
	case optionsFocusFit:
		m.toggleBackgroundFit()
	case optionsFocusAdjust:
		if m.options.hasBackground && m.options.framing.Fit == "fill" {
			m.options.editing = true
			m.options.framingFocus = framingFocusPreview
			m.options.framingBeforeEdit = m.options.framing
			m.requestOptions("")
		} else {
			m.options.status = "Choose an image and use Fill to adjust framing."
		}
	case optionsFocusPlayback:
		if m.options.playbackBlocked {
			m.options.userPaused = false
		} else {
			m.options.userPaused = !m.options.userPaused
		}
		m.options.playbackBlocked = false
		m.requestOptions("play")
	case optionsFocusPreview:
		m.requestOptions("preview")
	case optionsFocusTrim:
		m.options.trimming = true
		m.options.sourceFrame = !m.options.loopReady
		m.options.trimFocus = trimFocusSource
		m.requestOptions("")
	case optionsFocusApply:
		if m.options.newVideo && !m.options.loopReady {
			m.options.status = "Preview a loop before applying."
			return
		}
		m.options.status = "Saving…"
		m.requestOptions("apply")
	case optionsFocusRemoveBackground:
		m.options.status = "Removing…"
		m.requestOptions("remove")
	case optionsFocusBack:
		m.options.open = false
		if m.speechSetup.service != nil {
			m.options.open = true
			m.options.menu = true
		}
		m.requestOptions("")
	}
}

// The preview shrinks before the controls. Very short terminals omit it entirely.
func (m Model) optionsLayout() (int, PreviewRect) {
	if m.options.trimming {
		width := max(10, min(80, m.width-4))
		return width, PreviewRect{X: max(0, (m.width-width)/2) + 1, Y: 5, Width: width - 2, Height: max(0, min(12, m.height-33))}
	}
	if m.options.editing {
		width := max(10, min(80, m.width-4))
		return width, PreviewRect{X: max(0, (m.width-width)/2) + 1, Y: 4, Width: width - 2, Height: max(0, m.height-13)}
	}
	width := max(10, min(60, m.width-4))
	x := max(0, (m.width-width)/2)
	reserved := 24
	if m.options.newVideo && m.options.loopReady {
		reserved += 4
	}
	if m.options.video {
		reserved += m.videoOptionRows()
	}
	height := max(0, min(8, m.height-reserved))
	return width, PreviewRect{X: x + 1, Y: 8, Width: width - 2, Height: height}
}

func (m *Model) clickOption(x, y int, activate bool) {
	width, rect := m.optionsLayout()
	left := rect.X - 1
	if x < left || x >= left+width {
		return
	}
	shift := 0
	if rect.Height > 0 {
		shift = rect.Height + 2
	}
	switch {
	case y == 5 && activate:
		m.options.focus = optionsFocusChooseBackground
		m.activateOption()
	case y == 7+shift && activate:
		m.options.focus = optionsFocusFit
		m.activateOption()
	case y == 8+shift && activate:
		m.options.focus = optionsFocusAdjust
		m.activateOption()
	case y == 9+shift && activate:
		m.options.focus = optionsFocusDarkness
	case y == 10+shift:
		m.options.focus = optionsFocusDarkness
		m.options.darkness = max(0, min(95, (x-left)*95/max(1, width-1)))
	case y == 11+shift && activate:
		m.options.focus = optionsFocusQuality
		m.cycleVideoQuality()
	case m.options.newVideo && y == 15+shift && activate:
		m.options.focus = optionsFocusTrim
		m.activateOption()
	case m.options.video && y >= 12+shift && y <= 14+shift && activate:
		m.options.focus = optionsFocusSpeed + y - (12 + shift)
		if m.options.focus != optionsFocusSpeed {
			m.activateOption()
		}
	case y >= 13+shift+m.videoOptionRows() && y <= 15+shift+m.videoOptionRows() && activate:
		m.options.focus = optionsFocusApply + y - (13 + shift + m.videoOptionRows())
		m.activateOption()
	}
}

func (m Model) viewOptions() tea.View {
	if m.editorPreferences.open {
		return m.viewEditorPreferences()
	}
	if m.options.menu {
		return m.viewOptionsMenu()
	}
	if m.options.trimming {
		return m.viewVideoTrim()
	}
	if m.options.editing {
		return m.viewFraming()
	}
	width, rect := m.optionsLayout()
	selected := m.subjects.list.Styles.Title
	plain := lipgloss.NewStyle().Padding(0, 1)
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#999999"))
	label := func(index int, text string) string {
		if m.options.focus == index {
			return selected.Render(text)
		}
		return plain.Render(text)
	}
	lines := []string{selected.Render("Options"), "", "Visual", "", "Background", label(optionsFocusChooseBackground, "Choose image or video"), ""}
	if rect.Height > 0 {
		lines = append(lines, muted.Render("╭"+strings.Repeat("─", rect.Width)+"╮"))
		for range rect.Height {
			lines = append(lines, muted.Render("│")+strings.Repeat(" ", rect.Width)+muted.Render("│"))
		}
		lines = append(lines, muted.Render("╰"+strings.Repeat("─", rect.Width)+"╯"))
	}
	peak := m.options.darkness * (width - 1) / 95
	slider := strings.Repeat("━", peak) + "●" + strings.Repeat("─", width-peak-1)
	slider = muted.Render(slider)
	fitLabel := "Fill"
	if m.options.framing.Fit == "fit" {
		fitLabel = "Fit"
	}
	lines = append(lines, label(optionsFocusFit, "Background fit: "+fitLabel), label(optionsFocusAdjust, "Adjust framing"), label(optionsFocusDarkness, fmt.Sprintf("Background darkness: %d%%", m.options.darkness)), slider)
	lines = append(lines, label(optionsFocusQuality, "Video quality: "+m.options.videoQuality.Description()))
	if m.options.video {
		playbackLabel := "Pause video"
		if m.options.userPaused || m.options.playbackBlocked {
			playbackLabel = "Play video"
		}
		lines = append(lines, label(optionsFocusSpeed, fmt.Sprintf("Playback speed: %d%%", m.options.speedPercent)), label(optionsFocusPlayback, playbackLabel), label(optionsFocusPreview, "Play / pause preview"))
	}
	if m.options.newVideo {
		lines = append(lines, label(optionsFocusTrim, "Edit loop"))
	}
	lines = append(lines, "", label(optionsFocusApply, "Apply"), label(optionsFocusRemoveBackground, "Remove background"), label(optionsFocusBack, "Back"))
	if m.options.newVideo && m.options.loopReady {
		lines = append(lines, "", m.acceptedVideoLabel(), m.acceptedVideoRangeLabel(), m.draftVideoQualityLabel())
	}
	if rect.Height == 0 && m.height < 20 {
		lines = append(lines, muted.Render(m.options.status), muted.Render("↑/↓: focus · ←/→: value · Enter: activate · Q/Esc: back"))
	} else {
		lines = append(lines, "", muted.Render(m.options.status), "", muted.Render("↑/↓/Tab: focus · ←/→: value"), muted.Render("Enter: activate · Q/Esc: back"))
	}
	for i, line := range lines {
		lines[i] = strings.Repeat(" ", rect.X-1) + ansi.Truncate(line, width, "…")
	}
	view := tea.NewView(strings.Join(lines, "\n"))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

func (m Model) updateOptionsMenu(message tea.Msg) (tea.Model, tea.Cmd) {
	activate := false
	switch message := message.(type) {
	case tea.KeyPressMsg:
		switch message.String() {
		case "q", "esc":
			m.options.open = false
		case "tab", "down":
			m.options.menuFocus = (m.options.menuFocus + 1) % 4
		case "shift+tab", "up":
			m.options.menuFocus = (m.options.menuFocus + 3) % 4
		case "enter", "space":
			activate = true
		}
	case tea.MouseClickMsg:
		if message.Button == tea.MouseLeft && message.Y >= 2 && message.Y <= 5 {
			m.options.menuFocus = message.Y - 2
			activate = true
		}
	}
	if !activate {
		return m, nil
	}
	switch m.options.menuFocus {
	case 0:
		if !m.browserOptionsEnabled {
			return m, nil
		}
		m.options.menu = false
		m.options.trimming = false
		m.options.newVideo = false
		m.options.loopReady = false
		m.options.editing = false
		m.options.hasBackground = false
		m.options.framing = data.DefaultBackgroundFraming()
		m.options.focus = optionsFocusChooseBackground
		m.options.darkness = data.DefaultBackgroundDarkness
		m.options.speedPercent = 100
		m.options.userPaused = false
		m.options.status = "Loading visual…"
		m.requestOptions("load")
	case 1:
		if !m.browserOptionsEnabled {
			return m, nil
		}
		m.speechSetup.open = true
		m.speechSetup.offered = true
		m.speechSetup.focus = 0
	case 2:
		return m.openEditorPreferences()
	case 3:
		m.options.open = false
	}
	return m, nil
}
func (m Model) viewOptionsMenu() tea.View {
	rows := []string{m.subjects.list.Styles.Title.Render("Options"), ""}
	for index, label := range []string{"Visual", "Models", "Editor", "Back"} {
		if index == m.options.menuFocus {
			label = m.subjects.list.Styles.Title.Render(label)
		}
		rows = append(rows, label)
	}
	if !m.browserOptionsEnabled {
		rows = append(rows, "", "Visual and Models require browser mode.")
	}
	rows = append(rows, "", "↑/↓/Tab: focus · Enter: open · Esc: back")
	for index, row := range rows {
		rows[index] = ansi.Truncate(row, max(1, m.width), "…")
	}
	view := tea.NewView(strings.Join(rows, "\n"))
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	return view
}

func (m Model) videoOptionRows() int {
	if m.options.video {
		if m.options.newVideo {
			return 4
		}
		return 3
	}
	return 0
}

func (m Model) nextOptionsFocus(direction int) int {
	focus := m.options.focus
	for {
		focus = (focus + direction + optionsFocusCount) % optionsFocusCount
		if focus == optionsFocusTrim && !m.options.newVideo {
			continue
		}
		if m.options.video || focus < optionsFocusSpeed || focus > optionsFocusPreview {
			return focus
		}
	}
}

func (m *Model) setVideoQuality(key string) {
	m.options.status = "New previews only. Higher tiers can use more CPU."
	switch key {
	case "left":
		m.options.videoQuality = m.options.videoQuality.Step(-1)
	case "right":
		m.options.videoQuality = m.options.videoQuality.Step(1)
	case "home":
		m.options.videoQuality = visual.VideoQualityEfficient
	case "end":
		m.options.videoQuality = visual.VideoQualityMaximum
	}
}
func (m *Model) cycleVideoQuality() {
	m.options.status = "New previews only. Higher tiers can use more CPU."
	if m.options.videoQuality == visual.VideoQualityMaximum {
		m.options.videoQuality = visual.VideoQualityEfficient
	} else {
		m.options.videoQuality = m.options.videoQuality.Step(1)
	}
}
func (m Model) acceptedVideoLabel() string {
	return fmt.Sprintf("Previewed: %s · %d×%d · %s FPS", m.options.acceptedQuality.Label(), m.options.acceptedWidth, m.options.acceptedHeight, m.options.acceptedRate.Display())
}
func (m Model) acceptedVideoRangeLabel() string {
	startTimecode := strings.TrimSuffix(trimTimecode(m.options.acceptedTrim.StartMS), ".000")
	endTimecode := strings.TrimSuffix(trimTimecode(m.options.acceptedTrim.EndMS), ".000")
	playbackSeconds := float64(m.options.loopDurationMS) / float64(max(25, m.options.speedPercent)*10)
	return fmt.Sprintf("Range: %s - %s · plays %.2fs", startTimecode, endTimecode, playbackSeconds)
}
func (m Model) draftVideoQualityLabel() string {
	if m.options.videoQuality != m.options.acceptedQuality {
		return "Preview loop to use " + m.options.videoQuality.Label() + ". Apply uses " + m.options.acceptedQuality.Label() + "."
	}
	return "Quality applies to new previews."
}
