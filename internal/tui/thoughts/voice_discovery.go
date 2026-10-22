package thoughts

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/subjectdiscovery"
	"github.com/jhern254/go-thoughts/internal/thought"
	"github.com/jhern254/go-thoughts/internal/voice"
)

type subjectOrigin uint8

const (
	subjectUnset subjectOrigin = iota
	subjectAutomatic
	subjectExplicit
)

// The original text remains in suggestion.transcript until this recording is
// retired. Keep only its boundary and cleaned result to distinguish later edits.
type voicePrefixCleanup struct {
	recording voice.RecordingKey
	bodyStart int
	cleaned   string
}

// Draft-local recommendation state; recording boundaries retire cleanup permission.
type voiceSubjectSuggestion struct {
	snapshot          subjectdiscovery.Snapshot     // Preloaded names, aliases and usage; no reads during transcription.
	loaded            bool                          // Distinguishes pending data from a successfully loaded empty snapshot.
	origin            subjectOrigin                 // Explicit user/context choices take precedence over suggestions.
	outcome           subjectdiscovery.Outcome      // Last resolution result, used for the Subject field label.
	matches           []data.Subject                // Ranked shortlist for the deliberately opened picker.
	cue               string                        // Last evaluated cue; unchanged revisions need no reranking.
	transcript        string                        // Latest accepted contribution from this recording, before cleanup.
	commandEligible   bool                          // This recording may still supply the draft's initial command.
	beginningConsumed bool                          // Existing text or completed dictation makes later recordings literal.
	frozenCommand     subjectdiscovery.SpokenPrefix // Confirmed boundary retained at Stop for cleanup and late resolution.
	proposedName      string                        // Optional unknown name; must not block saving as Misc.
	proposedCommand   subjectdiscovery.SpokenPrefix // Uncertain boundary awaiting explicit Subject confirmation.
	cleanup           *voicePrefixCleanup           // One-shot F9 restoration; retired by restore or another recording.
}

// ConfigureVoiceSubjectDiscovery supplies the read-only coordinator at application
// composition. Recording updates only rank this draft's already-loaded snapshot.
func (m *Model) ConfigureVoiceSubjectDiscovery(service *subjectdiscovery.Service) {
	m.discovery = service
}

func (m *Model) beginSpokenSubject() {
	suggestion := &m.voice.suggestion
	suggestion.cleanup = nil
	suggestion.transcript = ""
	suggestion.cue = ""
	suggestion.frozenCommand = subjectdiscovery.SpokenPrefix{}
	// The optional field may survive another recording, but its old boundary
	// must not authorize cleanup of a new contribution.
	suggestion.proposedCommand = subjectdiscovery.SpokenPrefix{}
	// Only a recording at the beginning of an empty draft can contain a command.
	// Denied/empty attempts leave that opportunity available; later dictation is literal.
	if strings.TrimSpace(m.voice.recordingBaseText) != "" {
		suggestion.beginningConsumed = true
	}
	suggestion.commandEligible = !suggestion.beginningConsumed
}

func (m *Model) refreshSpokenSubject(candidatesChanged bool) {
	suggestion := &m.voice.suggestion
	if m.discovery == nil || m.voice.suspended {
		return
	}
	if suggestion.origin == subjectExplicit {
		// A contextual selection prevents automatic reassignment, but is not
		// evidence that the spoken cue is unambiguous. Deferred cleanup still
		// requires the snapshot and the frozen command from this recording.
		if candidatesChanged && m.voice.recordingStatus == "idle" && suggestion.frozenCommand.Cue != "" && suggestion.snapshot.ConfirmsCommand(suggestion.frozenCommand) {
			m.cleanUneditedSpokenPrefix(suggestion.frozenCommand)
		}
		return
	}
	cue := suggestion.frozenCommand.Cue
	proposal := false
	if suggestion.commandEligible {
		prefix, recognized := subjectdiscovery.ParseSpokenPrefix(suggestion.transcript)
		cue = ""
		if recognized && prefix.SingleWord && suggestion.loaded && suggestion.snapshot.MatchVoiceName(prefix.Cue).Outcome == subjectdiscovery.Unknown {
			cue, proposal = prefix.Cue, true
		} else if recognized && suggestion.snapshot.ConfirmsCommand(prefix) {
			cue = prefix.Cue
		}
	} else if !candidatesChanged || cue == "" {
		return
	} else if suggestion.snapshot.MatchVoiceName(cue).Outcome == subjectdiscovery.NameCollision {
		cue = ""
	} else if suggestion.frozenCommand.SingleWord && suggestion.snapshot.MatchVoiceName(cue).Outcome == subjectdiscovery.Unknown {
		proposal = true
	}
	if !candidatesChanged && suggestion.cue == cue {
		return
	}
	suggestion.cue = cue
	suggestion.proposedName = ""
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
	if proposal {
		suggestion.proposedName = cue
	} else if suggestion.loaded {
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
	// Until the snapshot arrives, a folded-name collision is unknown. Retain
	// the text at Stop; a late valid resolution can clean it only if unedited.
	if m.voice.recordingStatus == "idle" && suggestion.frozenCommand.Cue != "" && suggestion.snapshot.ConfirmsCommand(suggestion.frozenCommand) {
		m.cleanUneditedSpokenPrefix(suggestion.frozenCommand)
	}
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
		} else if recognized && suggestion.proposedName == prefix.Cue && suggestion.loaded && prefix.SingleWord && suggestion.snapshot.MatchVoiceName(prefix.Cue).Outcome == subjectdiscovery.Unknown {
			suggestion.proposedCommand = prefix
		}
	}
}

func (m *Model) finishSpokenSubject() {
	m.freezeSpokenSubject()
	suggestion := &m.voice.suggestion
	if suggestion.loaded && suggestion.frozenCommand.Cue != "" && suggestion.snapshot.ConfirmsCommand(suggestion.frozenCommand) {
		m.cleanUneditedSpokenPrefix(suggestion.frozenCommand)
	}
	if strings.TrimSpace(suggestion.transcript) != "" {
		suggestion.beginningConsumed = true
	}
}

// Subject creation remains explicit even when command intent permits cleanup.
// An optional proposal must not prevent saving the resulting Thought as Misc.
func (m Model) unconfirmedVoiceProposal() bool {
	return m.voice.suggestion.proposedName != "" && m.voice.query.Value() == m.voice.suggestion.proposedName
}

func (m *Model) confirmSpokenSubject() {
	suggestion := &m.voice.suggestion
	prefix := suggestion.proposedCommand
	if prefix.Cue == "" {
		prefix = suggestion.frozenCommand
	}
	suggestion.proposedCommand = subjectdiscovery.SpokenPrefix{}
	suggestion.proposedName = ""
	if prefix.Cue != "" {
		m.cleanUneditedSpokenPrefix(prefix)
	}
}

func (m *Model) cleanUneditedSpokenPrefix(prefix subjectdiscovery.SpokenPrefix) {
	// Subject confirmation can happen after editing or further dictation. Never
	// replace those edits with the earlier accepted recording contribution.
	if m.input.Value() != m.voice.suggestion.transcript {
		return
	}
	body := strings.TrimSpace(m.voice.suggestion.transcript[prefix.BodyStart:])
	// A prefix alone remains editable: stripping must not destroy all input.
	if body != "" {
		for offset, character := range body {
			if unicode.IsLetter(character) {
				body = body[:offset] + string(unicode.ToUpper(character)) + body[offset+len(string(character)):]
				break
			}
		}
		m.voice.suggestion.cleanup = &voicePrefixCleanup{
			recording: voice.RecordingKey{SessionID: m.browserSessionID, DraftID: m.voice.draftID, RecordingID: m.voice.recordingID},
			bodyStart: prefix.BodyStart,
			cleaned:   body,
		}
		m.input.SetValue(body)
		m.input.CursorEnd()
	}
}

func (m Model) canRestoreVoicePrefix() bool {
	cleanup := m.voice.suggestion.cleanup
	return m.VoiceOpen() && m.screen == create && !m.loading && !m.voice.suspended && !m.voiceLocked() &&
		cleanup != nil && cleanup.recording == (voice.RecordingKey{SessionID: m.browserSessionID, DraftID: m.voice.draftID, RecordingID: m.voice.recordingID})
}

func (m *Model) restoreVoicePrefix() {
	if !m.canRestoreVoicePrefix() {
		return
	}
	suggestion := &m.voice.suggestion
	cleanup := suggestion.cleanup
	restored := suggestion.transcript
	if m.input.Value() != cleanup.cleaned {
		restored = suggestion.transcript[:cleanup.bodyStart] + m.input.Value()
	}
	if utf8.RuneCountInString(restored) > thought.MaxThoughtCharacters || strings.Count(restored, "\n")+1 > textareaMaxLines {
		m.inputWarning = "Cannot restore prefix: the result exceeds the Thought or editor length limit. Draft unchanged."
		return
	}
	m.input.SetValue(restored)
	m.input.CursorEnd()
	m.inputWarning = ""
	// F9 intentionally restores transcript equality. Retire the permission itself,
	// rather than relying on the unedited-text check to reject delayed reads.
	suggestion.cleanup = nil
	suggestion.frozenCommand = subjectdiscovery.SpokenPrefix{}
	suggestion.proposedCommand = subjectdiscovery.SpokenPrefix{}
	suggestion.commandEligible = false
	suggestion.beginningConsumed = true
	suggestion.cue = ""
	suggestion.proposedName = ""
	if suggestion.origin == subjectAutomatic {
		m.subjectID = nil
		m.voice.subjectName = ""
		m.voice.query.SetValue("")
		m.voice.row = 0
		suggestion.origin = subjectUnset
		suggestion.matches = nil
		suggestion.outcome = subjectdiscovery.Unknown
	}
}

func (m Model) voiceSubjectLabel() string {
	if m.voice.suggestion.origin == subjectAutomatic {
		if m.unconfirmedVoiceProposal() {
			return "Subject (optional · proposed)"
		}
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
