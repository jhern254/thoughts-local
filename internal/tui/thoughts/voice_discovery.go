package thoughts

import (
	"strings"
	"unicode"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subjectdiscovery"
)

type subjectOrigin uint8

const (
	subjectUnset subjectOrigin = iota
	subjectAutomatic
	subjectExplicit
)

type voiceSubjectSuggestion struct {
	snapshot          subjectdiscovery.Snapshot
	loaded            bool
	origin            subjectOrigin
	outcome           subjectdiscovery.Outcome
	matches           []data.Subject
	cue               string
	transcript        string
	commandEligible   bool
	beginningConsumed bool
	frozenCommand     subjectdiscovery.SpokenPrefix
}

// ConfigureVoiceSubjectDiscovery supplies the read-only coordinator at application
// composition. Recording updates only rank this draft's already-loaded snapshot.
func (m *Model) ConfigureVoiceSubjectDiscovery(service *subjectdiscovery.Service) {
	m.discovery = service
}

func (m *Model) beginSpokenSubject() {
	suggestion := &m.voice.suggestion
	suggestion.transcript = ""
	suggestion.cue = ""
	suggestion.frozenCommand = subjectdiscovery.SpokenPrefix{}
	// Only a recording at the beginning of an empty draft can contain a command.
	// Denied/empty attempts leave that opportunity available; later dictation is literal.
	if strings.TrimSpace(m.voice.recordingBaseText) != "" {
		suggestion.beginningConsumed = true
	}
	suggestion.commandEligible = !suggestion.beginningConsumed
}

func (m *Model) refreshSpokenSubject(candidatesChanged bool) {
	suggestion := &m.voice.suggestion
	if m.discovery == nil || suggestion.origin == subjectExplicit || m.voice.suspended {
		return
	}
	cue := suggestion.frozenCommand.Cue
	if suggestion.commandEligible {
		prefix, recognized := subjectdiscovery.ParseSpokenPrefix(suggestion.transcript)
		cue = ""
		if recognized && suggestion.snapshot.ConfirmsCommand(prefix) {
			cue = prefix.Cue
		}
	} else if !candidatesChanged || cue == "" {
		return
	}
	if !candidatesChanged && suggestion.cue == cue {
		return
	}
	suggestion.cue = cue
	if suggestion.cue == "" {
		if suggestion.origin == subjectAutomatic {
			m.subjectID = nil
			m.voice.subjectName = ""
			m.voice.query.SetValue("")
			suggestion.origin = subjectUnset
			suggestion.matches = nil
		}
		return
	}
	result := subjectdiscovery.Result{Outcome: subjectdiscovery.Unknown}
	if suggestion.loaded {
		var err error
		result, err = m.discovery.Resolve(m.voice.ctx, suggestion.cue, suggestion.snapshot)
		if err != nil {
			return
		}
	}
	suggestion.origin = subjectAutomatic
	suggestion.outcome, suggestion.matches = result.Outcome, result.Matches
	m.subjectID = nil
	m.voice.subjectName = ""
	m.voice.query.SetValue(suggestion.cue)
	if result.Subject != nil {
		m.subjectID = &result.Subject.SubjectID
		m.voice.subjectName = result.Subject.SubjectName
		m.voice.query.SetValue(result.Subject.SubjectName)
	}
	m.voice.query.CursorEnd()
	m.voice.row = 0
}

// Freeze permission alongside transcript revocation. Candidate reads may still
// resolve this confirmed cue, but cannot promote uncertain speech after Stop.
// The accepted transcript and byte boundary remain owned by this recording until
// RecordingEnded; a new recording retires them in beginSpokenSubject.
func (m *Model) freezeSpokenSubject() {
	suggestion := &m.voice.suggestion
	if !suggestion.commandEligible {
		return
	}
	suggestion.commandEligible = false
	if m.discovery != nil {
		prefix, recognized := subjectdiscovery.ParseSpokenPrefix(suggestion.transcript)
		if recognized && suggestion.snapshot.ConfirmsCommand(prefix) {
			suggestion.frozenCommand = prefix
		}
	}
}

func (m *Model) finishSpokenSubject() {
	m.freezeSpokenSubject()
	suggestion := &m.voice.suggestion
	if suggestion.frozenCommand.Cue != "" {
		body := strings.TrimSpace(suggestion.transcript[suggestion.frozenCommand.BodyStart:])
		// A prefix alone remains editable: stripping must not destroy all input.
		if body != "" {
			for offset, character := range body {
				if unicode.IsLetter(character) {
					body = body[:offset] + string(unicode.ToUpper(character)) + body[offset+len(string(character)):]
					break
				}
			}
			m.input.SetValue(body)
			m.input.CursorEnd()
		}
	}
	if strings.TrimSpace(suggestion.transcript) != "" {
		suggestion.beginningConsumed = true
	}
}

func (m Model) voiceSubjectLabel() string {
	if m.voice.suggestion.origin == subjectAutomatic {
		switch m.voice.suggestion.outcome {
		case subjectdiscovery.Existing:
			return "Subject (optional · suggested)"
		case subjectdiscovery.Ambiguous:
			return "Subject (ambiguous · choose)"
		default:
			return "Subject (select or create)"
		}
	}
	return "Subject (optional)"
}
