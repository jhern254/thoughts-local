package browserterm

import (
	"encoding/json"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/tui"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
)

// The adapter observes authoritative state after Update, so queued commands
// cannot announce an obsolete draft. It carries bounded metadata and a one-use capability, never PCM or authored text.
type voiceBridgeModel struct {
	tea.Model
	writer         *terminalWriter
	audioSession   *audioSession
	lastVoiceState thoughts.VoiceState
}

func (m voiceBridgeModel) Init() tea.Cmd {
	return tea.Batch(m.Model.Init(), func() tea.Msg { return thoughts.BrowserRecordingEnabledMsg{SessionID: m.audioSession.sessionID} })
}
func (m voiceBridgeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.Model, cmd = m.Model.Update(msg)
	if model, ok := m.Model.(interface{ VoiceState() thoughts.VoiceState }); ok {
		voiceState := model.VoiceState()
		if voiceState.SessionID == "" && voiceState.BrowserRecordingEnabled {
			voiceState.SessionID = m.audioSession.sessionID
		}
		capability, rejectedRecording := m.audioSession.synchronizeRecording(voiceState)
		if rejectedRecording != nil {
			cmd = tea.Batch(cmd, func() tea.Msg { return *rejectedRecording })
		}
		if voiceState != m.lastVoiceState {
			m.lastVoiceState = voiceState
			controlMetadataJSON, _ := json.Marshal(struct {
				thoughts.VoiceState
				AudioCapability string `json:"audioCapability,omitempty"`
			}{voiceState, capability})
			_ = m.writer.writeFrame(append([]byte{'v'}, controlMetadataJSON...))
		}
	}
	return m, cmd
}

func (m voiceBridgeModel) OptionsState() tui.BrowserOptionsState {
	if model, ok := m.Model.(interface {
		OptionsState() tui.BrowserOptionsState
	}); ok {
		return model.OptionsState()
	}
	return tui.BrowserOptionsState{}
}
