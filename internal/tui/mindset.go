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
	// Day activity is an Events sub-focus, outside the Tab cycle.
	homePanelDays
)

type ThoughtService interface {
	thoughts.Service
	mindset.Reader
}

func (m *Model) resizeHome() {
	// Reserve the graph independently of asynchronous Mindset content. Otherwise
	// loading a longer reminder can resize the mountain between adjacent frames.
	m.days.Resize(m.width, m.height)
	m.mindsets.Resize(m.width, max(1, min(12, m.height-6-14-2-m.days.Height())))

	height := max(1, m.height-6)
	if m.events.TimelineVisible() {
		height = max(1, height-m.mindsets.Height()-2-m.days.Height())
	}
	if m.homeEventsHeight != height || m.homeEventsWidth != m.width {
		m.homeEventsHeight, m.homeEventsWidth = height, m.width
		m.events.Resize(m.width, height)
	}
}

func (m *Model) syncMindsets() tea.Cmd {
	var load tea.Cmd
	refresh := m.mindsetsNeedRefresh
	if revision := m.events.ActivityRevision(); revision != m.dayActivityRevision {
		m.dayActivityRevision = revision
		m.days.Invalidate()
	}
	if m.screen == screenEvents && m.events.DayReady() && (m.mindsetsNeedRefresh || !m.mindsets.Day().Equal(m.events.Day())) {
		load = m.mindsets.Load(m.events.Day())
		m.mindsetsNeedRefresh = false
	}
	visible := m.screen == screenEvents && m.events.TimelineVisible() && !m.exitPromptOpen && !m.speechSetup.open && !m.options.open
	var timer tea.Cmd
	m.mindsets, timer = m.mindsets.SetVisible(visible)
	m.resizeHome()
	var dayLoad tea.Cmd
	if m.events.DayReady() || !visible {
		dayLoad = m.days.Sync(m.events.Day(), visible, refresh)
	}
	return tea.Batch(load, timer, dayLoad)
}

func (m *Model) focusHome(panel homePanel) tea.Cmd {
	// Tab switches Mindset, Events and entities; arrows enter the day plot.
	m.entityFocused = panel == homePanelEntities
	m.days.SetFocused(panel == homePanelDays)
	m.days.SetHighlighted(panel != homePanelEvents)
	var cmd tea.Cmd
	m.mindsets, cmd = m.mindsets.SetFocused(panel == homePanelMindset)
	m.events.SetFocused(panel == homePanelEvents)
	m.events.SetDistributionHighlight(panel != homePanelEvents)
	return cmd
}
func (m Model) homeFocus() homePanel {
	if m.mindsets.Focused() {
		return homePanelMindset
	}
	if m.days.Focused() {
		return homePanelDays
	}
	if m.entityFocused {
		return homePanelEntities
	}
	return homePanelEvents
}
