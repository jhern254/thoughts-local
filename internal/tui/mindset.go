package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/tui/mindset"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
)

type homePanel uint8

const (
	homePanelMindset homePanel = iota
	homePanelEvents
	homePanelEntities
	homePanelCount
)

type ThoughtService interface {
	thoughts.Service
	mindset.Reader
}

func (m *Model) resizeHome() {
	m.mindsets.Resize(m.width, max(1, m.height-6-14-2))
	height := max(1, m.height-6)
	if m.events.TimelineVisible() {
		height = max(1, height-m.mindsets.Height()-2)
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

func (m *Model) focusHome(panel homePanel) tea.Cmd {
	// Focus follows the vertical order: Mindset, Events, entity strip.
	m.entityFocused = panel == homePanelEntities
	var cmd tea.Cmd
	m.mindsets, cmd = m.mindsets.SetFocused(panel == homePanelMindset)
	m.events.SetFocused(panel == homePanelEvents)
	return cmd
}
func (m Model) homeFocus() homePanel {
	if m.mindsets.Focused() {
		return homePanelMindset
	}
	if m.entityFocused {
		return homePanelEntities
	}
	return homePanelEvents
}
