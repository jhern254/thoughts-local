package thoughts

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/failure"
	"github.com/jhern254/go-thoughts/internal/logging"
)

// BrowserRecordingEnabledMsg enables recording controls for a browser session.
// Only the browser adapter sends it. Microphone support and permission are
// checked on Record, not when the adapter enables these controls.
type BrowserRecordingEnabledMsg struct{}

// VoiceAction contains control metadata only, never audio or transcript text.
type VoiceAction struct {
	Action      string `json:"action"`
	DraftID     uint64 `json:"draftID"`
	RecordingID uint64 `json:"recordingID"`
}

// VoiceState reports browser recording controls and the current draft's status.
type VoiceState struct {
	// BrowserRecordingEnabled means this session uses the browser adapter.
	// It does not imply microphone permission, API support, or a working device.
	BrowserRecordingEnabled bool `json:"browserRecordingEnabled"`
	// CanOpenThoughtDraft means the current screen permits quick thought entry
	// without interrupting an active form or filter.
	CanOpenThoughtDraft bool `json:"canOpenThoughtDraft"`
	// DraftID is zero when no draft is accepting browser recording controls.
	DraftID uint64 `json:"draftID"`
	// RecordingID identifies an attempt within the draft, rejecting late replies
	// from earlier attempts. Zero means recording has not been attempted yet.
	RecordingID uint64 `json:"recordingID"`
	// RecordingStatus is idle, requesting, recording, or stopping; it is empty
	// when no draft is accepting recording controls.
	RecordingStatus string `json:"recordingStatus"`
}

type VoiceSubjectReader interface {
	List(context.Context, string) ([]data.Subject, error)
}

// VoiceSubjectRequest preserves identity while the root creates a subject.
type VoiceSubjectRequest struct {
	Query   string
	owner   *int
	draftID uint64
}
type voiceSubjectsLoaded struct {
	owner             *int
	draftID, revision uint64
	items             []data.Subject
	err               error
}
type voiceDraft struct {
	ctx                            context.Context
	cancel                         context.CancelFunc
	cancelRead                     context.CancelFunc
	draftID, recordingID, revision uint64
	recordingStatus, message       string
	suspended, subjectFocused      bool
	query                          textinput.Model
	items                          []data.Subject
	row, width, height             int
	loading                        bool
	subjectName                    string
	originalSubjectID              *int64
}

func (m *Model) stopVoice() {
	if m.voice.cancel != nil {
		m.voice.cancel()
	}
	if m.voice.cancelRead != nil {
		m.voice.cancelRead()
	}
	m.voice = voiceDraft{}
}
func (m *Model) OpenVoice(draftID uint64, reader VoiceSubjectReader) tea.Cmd {
	m.Reset()
	m.screen = create
	return tea.Batch(m.input.Focus(), m.EnableVoice(draftID, reader, ""))
}

// EnableVoice equips the existing editor without replacing its text or subject.
func (m *Model) EnableVoice(draftID uint64, reader VoiceSubjectReader, subjectName string) tea.Cmd {
	ctx, cancel := context.WithCancel(m.ctx)
	query := textinput.New()
	query.Placeholder = "Misc"
	query.CharLimit = 0
	query.SetValue(subjectName)
	query.CursorEnd()
	m.voice = voiceDraft{
		ctx:               ctx,
		cancel:            cancel,
		draftID:           draftID,
		recordingStatus:   "idle",
		query:             query,
		width:             m.list.Width(),
		height:            m.list.Height() + 4,
		subjectName:       subjectName,
		originalSubjectID: m.subjectID,
	}
	m.resizeVoice(m.voice.width, m.voice.height)
	return m.loadVoiceSubjects(reader)
}
func (m Model) VoiceState() VoiceState {
	state := VoiceState{
		DraftID:         m.voice.draftID,
		RecordingID:     m.voice.recordingID,
		RecordingStatus: m.voice.recordingStatus,
	}
	if m.voice.suspended || m.screen != create || m.loading {
		state.DraftID = 0
		state.RecordingStatus = ""
	}
	return state
}
func (m Model) VoiceOpen() bool { return m.voice.draftID != 0 }
func (m Model) voiceLocked() bool {
	return m.voice.recordingStatus == "requesting" || m.voice.recordingStatus == "recording" || m.voice.recordingStatus == "stopping"
}
func (m *Model) resizeVoice(width, height int) {
	m.voice.width, m.voice.height = max(1, width), max(1, height)
	m.voice.query.SetWidth(max(1, width-2))
	reserved := 5
	if m.voice.subjectFocused {
		reserved += 4
	}
	m.input.SetHeight(max(1, height-reserved))
}
func (m *Model) loadVoiceSubjects(reader VoiceSubjectReader) tea.Cmd {
	if m.voice.cancelRead != nil {
		m.voice.cancelRead()
	}
	ctx, cancel := context.WithCancel(m.voice.ctx)
	m.voice.cancelRead = cancel
	m.voice.revision++
	m.voice.loading = true
	owner, draftID, revision, user := m.owner, m.voice.draftID, m.voice.revision, m.userID
	return func() tea.Msg {
		if ctx.Err() != nil {
			return nil
		}
		items, err := reader.List(ctx, user)
		if ctx.Err() != nil {
			return nil
		}
		return voiceSubjectsLoaded{owner, draftID, revision, items, err}
	}
}
func (m Model) updateVoiceAction(action VoiceAction) (Model, tea.Cmd) {
	if m.voice.draftID == 0 || m.voice.suspended || m.screen != create || m.loading || action.DraftID != m.voice.draftID || action.RecordingID != m.voice.recordingID {
		return m, nil
	}
	switch action.Action {
	case "start":
		if m.voiceLocked() {
			return m, nil
		}
		m.stopClipboard()
		m.request++
		m.voice.recordingID++
		m.voice.recordingStatus = "requesting"
		m.voice.message = ""
		m.input.Blur()
		m.voice.query.Blur()
	case "recording":
		if m.voice.recordingStatus == "requesting" {
			m.voice.recordingStatus = "recording"
		}
	case "stop":
		if m.voice.recordingStatus == "requesting" || m.voice.recordingStatus == "recording" {
			m.voice.recordingStatus = "stopping"
		}
	case "stopped", "denied", "unavailable", "failed":
		if !m.voiceLocked() {
			return m, nil
		}
		m.voice.recordingStatus = "idle"
		m.voice.message = map[string]string{"denied": "Microphone permission was denied. Draft unchanged.", "unavailable": "Microphone unavailable. Draft unchanged.", "failed": "Microphone capture failed. Draft unchanged."}[action.Action]
		m.voice.subjectFocused = false
		m.resizeVoice(m.voice.width, m.voice.height)
		return m, m.input.Focus()
	}
	return m, nil
}
func (m Model) matchingVoiceSubjects() []data.Subject {
	query := strings.ToLower(m.voice.query.Value())
	items := []data.Subject{}
	for _, item := range m.voice.items {
		if strings.Contains(strings.ToLower(item.SubjectName), query) {
			items = append(items, item)
		}
	}
	return items
}
func (m Model) AwaitingVoiceSubject(request VoiceSubjectRequest) bool {
	return m.voice.draftID != 0 && m.voice.suspended && request.owner == m.owner && request.draftID == m.voice.draftID
}
func (m *Model) ReturnFromVoiceSubject(request VoiceSubjectRequest, subject *data.Subject) tea.Cmd {
	if !m.AwaitingVoiceSubject(request) {
		return nil
	}
	m.voice.suspended = false
	if subject != nil {
		if m.voice.cancelRead != nil {
			m.voice.cancelRead()
		}
		m.voice.revision++
		m.voice.loading = false
		m.voice.items = append(m.voice.items, *subject)
		m.subjectID = &subject.SubjectID
		m.voice.subjectName = subject.SubjectName
		m.voice.query.SetValue(subject.SubjectName)
		m.voice.query.CursorEnd()
		m.voice.subjectFocused = false
		m.resizeVoice(m.voice.width, m.voice.height)
		return m.input.Focus()
	}
	return m.voice.query.Focus()
}
func (m Model) updateVoicePicker(msg tea.Msg) (Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "up", "down":
			step := 1
			if key.String() == "up" {
				step = -1
			}
			m.voice.row = max(0, min(m.voice.row+step, len(m.matchingVoiceSubjects())))
			return m, nil
		case "enter":
			if m.voice.row == 0 {
				m.voice.suspended = true
				m.voice.query.Blur()
				request := VoiceSubjectRequest{m.voice.query.Value(), m.owner, m.voice.draftID}
				return m, func() tea.Msg { return request }
			}
			item := m.matchingVoiceSubjects()[m.voice.row-1]
			m.subjectID = &item.SubjectID
			m.voice.subjectName = item.SubjectName
			m.voice.query.SetValue(item.SubjectName)
			m.voice.query.CursorEnd()
			m.voice.subjectFocused = false
			m.voice.query.Blur()
			m.voice.row = 0
			m.resizeVoice(m.voice.width, m.voice.height)
			return m, m.input.Focus()
		}
	}
	var text string
	switch msg := msg.(type) {
	case tea.PasteMsg:
		text = msg.Content
	case tea.KeyPressMsg:
		text = msg.Text
	}
	if !utf8.ValidString(text) {
		m.inputWarning = "Input rejected: invalid Unicode. Draft unchanged."
		return m, nil
	}
	for _, r := range text {
		if r == utf8.RuneError || unicode.IsControl(r) {
			m.inputWarning = "Input rejected: subject search requires plain text. Draft unchanged."
			return m, nil
		}
	}
	if text != "" {
		m.inputWarning = ""
	}
	var cmd tea.Cmd
	before := m.voice.query.Value()
	m.voice.query, cmd = m.voice.query.Update(msg)
	if m.voice.query.Value() != before {
		m.subjectID = nil
		m.voice.subjectName = ""
		m.voice.message = ""
		m.voice.row = 0
	}
	return m, cmd
}
func (m Model) acceptVoiceSubjects(reply voiceSubjectsLoaded) (Model, tea.Cmd) {
	if reply.owner != m.owner || reply.draftID != m.voice.draftID || reply.revision != m.voice.revision || m.voice.draftID == 0 {
		return m, nil
	}
	m.voice.loading = false
	if reply.err != nil {
		if category, emit := failure.Classify(logging.SubjectList, reply.err); emit {
			m.logger.Failure(logging.SubjectList, category)
		}
		m.voice.message = "Could not load subjects. You can still create one."
	} else {
		m.voice.items = reply.items
	}
	return m, nil
}

// editorView is shared by ordinary creation and browser recording.
func (m Model) editorView(status string) string {
	if m.inputWarning != "" {
		status = m.inputWarning + "\n"
	} else if m.voice.message != "" {
		status = m.voice.message + "\n"
	}
	body := "Create thought\n" + status + m.input.View()
	help := "Ctrl+S: save • Enter: newline • Esc: cancel"
	if m.VoiceOpen() {
		body += "\nSubject (optional)\n" + m.voice.query.View()
		if m.voice.subjectFocused {
			items := m.matchingVoiceSubjects()
			visible := max(1, min(4, m.voice.height-8))
			top := max(0, m.voice.row-visible+1)
			for row := top; row < min(top+visible, len(items)+1); row++ {
				label := "Create subject…"
				if row > 0 {
					label = items[row-1].SubjectName
				}
				prefix := "    "
				if row == m.voice.row {
					prefix = "  > "
				}
				body += "\n" + ansi.Truncate(prefix+label, max(1, m.voice.width), "…")
			}
		}
		action := "F8: Record"
		if m.voiceLocked() {
			action = "F8: Stop"
		}
		help = action + " • Tab: subject/text • " + help
		if m.voiceLocked() {
			help = action + " • Esc: cancel"
		}
		body += "\n" + ansi.Wrap(help, max(1, m.voice.width), "")
		return body
	}
	return body + "\n" + help
}
