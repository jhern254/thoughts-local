package browserterm

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/tui"
)

type appearanceReceiver struct {
	enabled      bool
	borderWeight data.BorderWeight
}

func (m appearanceReceiver) Init() tea.Cmd  { return nil }
func (m appearanceReceiver) View() tea.View { return tea.NewView("") }
func (m appearanceReceiver) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tui.BrowserOptionsEnabledMsg:
		m.enabled = true
	case tui.BrowserOptionsMsg:
		if m.enabled && msg.Action == "appearance" {
			m.borderWeight = msg.BorderWeight
		}
	}
	return m, nil
}
func TestOptionsBridge_StartupAppearance(t *testing.T) {
	t.Run("accepts saved appearance before asynchronous Init results arrive", func(t *testing.T) {
		bridge := optionsBridgeModel{Model: appearanceReceiver{}}
		bridge.Init()
		updated, _ := bridge.Update(tui.BrowserOptionsMsg{Action: "appearance", BorderWeight: data.BorderHeavy})
		got := updated.(optionsBridgeModel).Model.(appearanceReceiver)
		if got.borderWeight != data.BorderHeavy {
			t.Fatal("startup appearance was dropped before browser capabilities were enabled")
		}
	})
}
