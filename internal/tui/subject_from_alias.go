package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/diagnostics"
)

func (m Model) subjectCreationAliasConflict() *data.SubjectAliasConflictError {
	var conflict *data.SubjectAliasConflictError
	if errors.As(diagnostics.SingleError(m.subjects.err), &conflict) && conflict.AliasID > 0 && conflict.AliasName == m.subjects.input.Value() {
		return conflict
	}
	return nil
}

func (m Model) updateSubjectFromAlias(message tea.Msg) (tea.Model, tea.Cmd) {
	if m.subjects.loading {
		return m, nil
	}
	key, ok := message.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc", "q", "n":
		m.screen = screenSubjectCreate
		m.subjects.aliasToReplace = nil
		m.subjects.err = nil
		return m, m.subjects.input.Focus()
	case "y":
		conflict := m.subjects.aliasToReplace
		if conflict == nil {
			return m, nil
		}
		m.subjects.createSession = new(int)
		m.subjects.loading = true
		m.subjects.err = nil
		ctx, service, userID := m.ctx, m.subjects.service, m.user.UserID
		session := m.subjects.createSession
		// Like ordinary Subject creation, a confirmed write belongs to the app
		// session. Wait for its result before navigation; cancellation cannot undo
		// a transaction that has already committed.
		return m, func() tea.Msg {
			created, err := service.CreateFromAlias(ctx, userID, conflict.SubjectID, conflict.AliasID, conflict.AliasName)
			return subjectCreatedMsg{session: session, subject: created, err: err}
		}
	}
	return m, nil
}

func (m Model) viewSubjectFromAlias() string {
	conflict := m.subjects.aliasToReplace
	if conflict == nil {
		return "Alias unavailable\n\nEsc: back"
	}
	width := max(1, m.width)
	rows := []string{
		"Create subject",
		"",
		ansi.Truncate(fmt.Sprintf("Alias: %q", conflict.AliasName), width, "…"),
		ansi.Truncate(fmt.Sprintf("From: %q", conflict.SubjectName), width, "…"),
		"",
		"Remove this alias and create a new empty Subject?",
		"",
		"Existing Thoughts, Events and Goal links stay with the original Subject.",
		"",
	}
	if m.subjects.loading {
		rows = append(rows, "Creating subject…")
	} else {
		if m.subjects.err != nil {
			feedback := diagnostics.SubjectMessage(m.subjects.err, "Could not create the subject.")
			if errors.Is(diagnostics.SingleError(m.subjects.err), data.ErrRecordNotFound) {
				feedback = "The alias changed or is unavailable. Go back and try again."
			}
			rows = append(rows, "Error: "+feedback, "")
		}
		rows = append(rows, "y: remove alias and create", "n/Q/Esc: back")
	}
	return ansi.Wrap(strings.Join(rows, "\n"), width, "")
}
