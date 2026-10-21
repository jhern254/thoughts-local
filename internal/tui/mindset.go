package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/tui/mindset"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
)

type ThoughtService interface {
	thoughts.Service
	mindset.Reader
}

func (m *Model) resizeHome() {
	m.mindsets.Resize(m.width, max(1, m.height-6-14-2))
	height := max(1, m.height-6)
	if m.events.TimelineVisible() {
		height = max(1, height-m.mindsets.Height()-1)
	}
	if m.homeEventsHeight != height || m.homeEventsWidth != m.width {
		m.homeEventsHeight, m.homeEventsWidth = height, m.width
		m.events.Resize(m.width, height)
	}
}

func (m *Model) syncMindsets() tea.Cmd {
	var load tea.Cmd
	if m.screen == screenEvents && m.events.DayReady() && (m.mindsetsNeedRefresh || !m.mindsets.Day().Equal(m.events.Day())) {
		load = m.mindsets.Load(m.events.Day())
		m.mindsetsNeedRefresh = false
	}
	visible := m.screen == screenEvents && m.events.TimelineVisible() && !m.exitPromptOpen && !m.speechSetup.open && !m.options.open
	var timer tea.Cmd
	m.mindsets, timer = m.mindsets.SetVisible(visible)
	m.resizeHome()
	return tea.Batch(load, timer)
}

func (m *Model) focusHome(panel int) tea.Cmd {
	// Focus follows the vertical order: Mindset, Events, entity strip.
	m.entityFocused = panel == 2
	var cmd tea.Cmd
	m.mindsets, cmd = m.mindsets.SetFocused(panel == 0)
	m.events.SetFocused(panel == 1)
	return cmd
}
func (m Model) homeFocus() int {
	if m.mindsets.Focused() {
		return 0
	}
	if m.entityFocused {
		return 2
	}
	return 1
}
