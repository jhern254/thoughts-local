package goals

import (
	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/tui/appearance"
)

const activityPageDots = 10

// Crossing the loaded range fetches the adjacent Event window. Only that window
// stays in memory; ordinary movement within it needs no database read.
func (m *Model) updateActivityList(msg tea.Msg) tea.Cmd {
	msg = appearance.ListScrollInput(msg)
	if key, ok := msg.(tea.KeyPressMsg); ok {
		pager := m.activity.list.Paginator
		index := m.activity.list.Index()
		last := max(0, len(m.activity.list.Items())-1)
		older, newer := false, false
		switch key.String() {
		case "down", "j":
			older = index == last
		case "right", "l", "pgdown":
			older = pager.OnLastPage()
		case "up", "k":
			newer = index == 0
		case "left", "h", "pgup":
			newer = pager.OnFirstPage()
		}
		if older && m.activity.eventsOlder || newer && m.activity.eventsNewer {
			if m.activity.loading {
				return nil
			}
			direction := data.EarnedEventsOlder
			if len(m.activity.events.Events) == 0 {
				// Concurrent deletion can empty the next window. Restart at
				// the newest remaining Events rather than lose the way back.
				return m.loadActivityEvents(data.GoalEventPageRequest{})
			}
			var cursor *data.GoalEventCursor
			if newer {
				direction = data.EarnedEventsNewer
			}
			if events := m.activity.events.Events; len(events) > 0 {
				item := events[len(events)-1]
				if newer {
					item = events[0]
				}
				boundary := item.Cursor()
				cursor = &boundary
			}
			row := m.activity.list.Cursor()
			switch key.String() {
			case "down", "j":
				row = 0
			case "up", "k":
				row = pager.PerPage - 1
			}
			cmd := m.loadActivityEvents(data.GoalEventPageRequest{Cursor: cursor, Direction: direction})
			m.activity.eventPageRow = row
			return cmd
		}
	}
	var cmd tea.Cmd
	m.activity.list, cmd = m.activity.list.Update(msg)
	return cmd
}
