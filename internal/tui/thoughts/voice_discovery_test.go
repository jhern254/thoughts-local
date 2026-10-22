package thoughts

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/subjectdiscovery"
	"github.com/jhern254/go-thoughts/internal/voice"
)

type discoverySubjects struct {
	items    []data.Subject
	aliases  []data.SubjectAlias
	calls    int
	aliasErr error
	listErr  error
	ctx      context.Context
}

func (s *discoverySubjects) List(ctx context.Context, _ string) ([]data.Subject, error) {
	s.calls++
	s.ctx = ctx
	return s.items, s.listErr
}
func (s *discoverySubjects) ListAliasesForUser(context.Context, string) ([]data.SubjectAlias, error) {
	s.calls++
	return s.aliases, s.aliasErr
}

type discoveryUsage struct{}

func (discoveryUsage) ThoughtCountsBySubject(context.Context, string) ([]data.SubjectThoughtCountView, error) {
	return nil, nil
}

func discoveryModel(t *testing.T) (Model, tea.Cmd, *discoverySubjects) {
	t.Helper()
	m := voiceModel(t)
	m, _ = m.Update(BrowserVoiceSessionMsg{SessionID: "session", SpeechRecognitionAvailable: true})
	subjects := &discoverySubjects{
		items: []data.Subject{
			{SubjectID: 7, SubjectName: "Go Programming"},
			{SubjectID: 8, SubjectName: "Fitness"},
		},
		aliases: []data.SubjectAlias{{UserID: "u", SubjectID: 7, AliasName: "Programming in Go"}},
	}
	m.ConfigureVoiceSubjectDiscovery(subjectdiscovery.NewService(subjects, discoveryUsage{}, subjectdiscovery.NameRecommender{}))
	cmd := m.loadVoiceSubjects(subjects)
	return m, cmd, subjects
}
func startDiscoveryRecording(m Model) (Model, voice.RecordingKey) {
	m, _ = m.Update(VoiceAction{Action: "start", DraftID: 1, RecordingID: m.voice.recordingID})
	return m, m.VoiceState().TranscriptAuthority.Recording()
}
func transcript(m Model, recording voice.RecordingKey, revision uint64, text string) Model {
	m, _ = m.Update(voice.TranscriptUpdate{Recording: recording, Revision: revision, Text: text})
	return m
}
func TestModel_VoiceDiscovery(t *testing.T) {
	t.Run("narrative remains literal and saveable even when its prefix is an exact alias", func(t *testing.T) {
		for _, text := range []string{
			"Yesterday I thought about Go.",
			"Last night I thought about work.",
			"My first thought was to practice.",
			"It was thought to be finished.",
			"I suddenly thought about something.",
			"For a moment I thought it worked.",
			"Today I thought about my goals.",
			"Earlier we thought about moving.",
			"The first thought I had was interesting.",
			"I really thought this would work.",
			"Yesterday, I thought，about 日本語.",
		} {
			t.Run(text, func(t *testing.T) {
				m, _, subjects := discoveryModel(t)
				cue, _, _ := strings.Cut(text, " thought")
				subjects.aliases = append(subjects.aliases, data.SubjectAlias{UserID: "u", SubjectID: 7, AliasName: cue})
				m, _ = m.Update(m.loadVoiceSubjects(subjects)())
				stamp := m.draftObservedAt
				m, recording := startDiscoveryRecording(m)
				m = transcript(m, recording, 1, text)
				if m.subjectID != nil || m.voice.query.Value() != "" || m.input.Value() != text {
					t.Fatalf("live subject=%v query=%q text=%q, want literal unassigned dictation", m.subjectID, m.voice.query.Value(), m.input.Value())
				}
				m, _ = m.Update(voice.RecordingEnded{Recording: recording})
				m, _ = m.Update(voice.RecordingEnded{Recording: recording})
				if m.input.Value() != text || m.draftObservedAt != stamp {
					t.Fatalf("ended text=%q, want unchanged %q and opening time", m.input.Value(), text)
				}
				_, save := m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
				if save == nil {
					t.Fatal("ordinary speech blocked Misc save")
				}
				result := save().(Result)
				if result.err != nil || result.item.SubjectID != nil || result.item.Thought != text || !result.item.ObservedAt.Equal(stamp) {
					t.Fatalf("saved=%+v error=%v, want literal Misc Thought at opening time", result.item, result.err)
				}
			})
		}
	})
	t.Run("one-word proposal bypasses ranking and remains saveable as Misc", func(t *testing.T) {
		m, _, subjects := discoveryModel(t)
		ranker := &countedRecommender{}
		m.ConfigureVoiceSubjectDiscovery(subjectdiscovery.NewService(subjects, discoveryUsage{}, ranker))
		m, _ = m.Update(m.loadVoiceSubjects(subjects)())
		m, recording := startDiscoveryRecording(m)
		text := "random thought, perhaps I should learn Go."
		m = transcript(m, recording, 1, text)
		m, _ = m.Update(voice.RecordingEnded{Recording: recording})
		if ranker.calls != 0 || m.subjectID != nil || m.voice.query.Value() != "random" || m.input.Value() != text {
			t.Fatalf("ranking=%d subject=%v query=%q text=%q, want untouched uncertain speech", ranker.calls, m.subjectID, m.voice.query.Value(), m.input.Value())
		}
		_, save := m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
		if save == nil || save().(Result).err != nil {
			t.Fatal("uncertain speech blocked Misc save")
		}
	})
	t.Run("body revision withdraws exact-name confirmation before final cleanup", func(t *testing.T) {
		m, _, subjects := discoveryModel(t)
		subjects.aliases = append(subjects.aliases, data.SubjectAlias{UserID: "u", SubjectID: 7, AliasName: "The first"})
		m, _ = m.Update(m.loadVoiceSubjects(subjects)())
		m, recording := startDiscoveryRecording(m)
		m = transcript(m, recording, 1, "The first thought")
		if m.subjectID == nil || *m.subjectID != 7 {
			t.Fatal("possible exact alias did not confirm live")
		}
		final := "The first thought I had was interesting."
		m = transcript(m, recording, 2, final)
		if m.subjectID != nil || m.voice.query.Value() != "" {
			t.Fatal("body revision retained provisional command with unchanged cue")
		}
		m, _ = m.Update(VoiceAction{Action: "stop", DraftID: 1, RecordingID: 1})
		m = transcript(m, recording, 3, "Go Programming thought, late command.")
		m, _ = m.Update(voice.RecordingEnded{Recording: recording})
		if m.input.Value() != final || m.subjectID != nil {
			t.Fatalf("got %q subject=%v, want final accepted literal dictation", m.input.Value(), m.subjectID)
		}
	})
	t.Run("delayed exact-name evidence cannot revive a revised narrative", func(t *testing.T) {
		m, _, subjects := discoveryModel(t)
		subjects.aliases = append(subjects.aliases, data.SubjectAlias{UserID: "u", SubjectID: 7, AliasName: "The first"})
		reply := m.loadVoiceSubjects(subjects)()
		m, recording := startDiscoveryRecording(m)
		m = transcript(m, recording, 1, "The first thought")
		text := "The first thought I had was interesting."
		m = transcript(m, recording, 2, text)
		m, _ = m.Update(voice.RecordingEnded{Recording: recording})
		m, _ = m.Update(reply)
		if m.subjectID != nil || m.voice.query.Value() != "" || m.input.Value() != text {
			t.Fatalf("subject=%v query=%q text=%q, want unchanged unassigned final narrative", m.subjectID, m.voice.query.Value(), m.input.Value())
		}
	})
	t.Run("sentence-like names resolve live and cleanup capitalizes only the body", func(t *testing.T) {
		for _, name := range []string{"The Art of Learning", "I Ching", "I Want to Learn Go"} {
			t.Run(name, func(t *testing.T) {
				m, _, subjects := discoveryModel(t)
				subjects.aliases = append(subjects.aliases, data.SubjectAlias{UserID: "u", SubjectID: 7, AliasName: name})
				m, _ = m.Update(m.loadVoiceSubjects(subjects)())
				m, recording := startDiscoveryRecording(m)
				text := name + " thought, étudier les interfaces."
				m = transcript(m, recording, 1, text)
				if m.subjectID == nil || *m.subjectID != 7 || m.voice.query.Value() != "Go Programming" || m.input.Value() != text {
					t.Fatalf("live subject=%v query=%q text=%q, want canonical alias selection and full transcript", m.subjectID, m.voice.query.Value(), m.input.Value())
				}
				m, _ = m.Update(voice.RecordingEnded{Recording: recording})
				if got, want := m.input.Value(), "Étudier les interfaces."; got != want {
					t.Fatalf("body=%q, want %q", got, want)
				}
			})
		}
	})
	t.Run("recommends canonical alias live and strips only after recording ends", func(t *testing.T) {
		m, cmd, subjects := discoveryModel(t)
		m, _ = m.Update(cmd())
		m, recording := startDiscoveryRecording(m)
		text := "Programming in Go thought, today I learned about interfaces."
		m = transcript(m, recording, 1, text)
		if m.subjectID == nil || *m.subjectID != 7 || m.voice.query.Value() != "Go Programming" || m.input.Value() != text {
			t.Fatalf("live subject=%v query=%q text=%q", m.subjectID, m.voice.query.Value(), m.input.Value())
		}
		m = transcript(m, recording, 2, text+" And generics.")
		m, _ = m.Update(VoiceAction{Action: "stop", DraftID: 1, RecordingID: 1})
		m = transcript(m, recording, 3, "Fitness thought late")
		if m.input.Value() != text+" And generics." {
			t.Fatal("Stop did not freeze latest transcript")
		}
		m, _ = m.Update(voice.RecordingEnded{Recording: recording})
		if got, want := m.input.Value(), "Today I learned about interfaces. And generics."; got != want {
			t.Fatalf("got %q want %q", got, want)
		}
		m, _ = m.Update(voice.RecordingEnded{Recording: recording})
		if subjects.calls != 2 {
			t.Fatalf("read calls=%d want 2", subjects.calls)
		}
		m, second := startDiscoveryRecording(m)
		m = transcript(m, second, 1, "Fitness thought literal second recording")
		m, _ = m.Update(voice.RecordingEnded{Recording: second})
		if *m.subjectID != 7 || !strings.HasSuffix(m.input.Value(), "\nFitness thought literal second recording") {
			t.Fatalf("second recording altered subject/text: %q", m.input.Value())
		}
	})
	t.Run("revised cue replaces provisional selection and missing command clears it", func(t *testing.T) {
		m, cmd, _ := discoveryModel(t)
		m, _ = m.Update(cmd())
		m, r := startDiscoveryRecording(m)
		m = transcript(m, r, 1, "Programming in Go thought hello")
		m = transcript(m, r, 2, "Fitness thought hello")
		if m.subjectID == nil || *m.subjectID != 8 {
			t.Fatal("revised cue did not replace suggestion")
		}
		m = transcript(m, r, 3, "I thought this was interesting")
		m, _ = m.Update(voice.RecordingEnded{Recording: r})
		if m.subjectID != nil || m.voice.query.Value() != "" || m.input.Value() != "I thought this was interesting" {
			t.Fatal("no command did not retain unassigned text")
		}
	})
	t.Run("late candidates resolve latest accepted cue even after Stop", func(t *testing.T) {
		m, cmd, _ := discoveryModel(t)
		reply := cmd()
		m, r := startDiscoveryRecording(m)
		m = transcript(m, r, 1, "Fitness thought old")
		m = transcript(m, r, 2, "Programming in Go thought, 界hello")
		m, _ = m.Update(voice.RecordingEnded{Recording: r})
		m, _ = m.Update(reply)
		if m.subjectID == nil || *m.subjectID != 7 || m.input.Value() != "界hello" {
			t.Fatalf("late result=%v %q", m.subjectID, m.input.Value())
		}
	})
	t.Run("manual edits win over delayed data", func(t *testing.T) {
		m, cmd, _ := discoveryModel(t)
		reply := cmd()
		m, r := startDiscoveryRecording(m)
		m = transcript(m, r, 1, "Programming in Go thought hello")
		m, _ = m.Update(voice.RecordingEnded{Recording: r})
		m, _ = m.Update(key(tea.KeyTab))
		m, _ = m.Update(tea.PasteMsg{Content: " custom"})
		before := m.voice.query.Value()
		m, _ = m.Update(reply)
		if m.voice.query.Value() != before || m.subjectID != nil {
			t.Fatal("late data overwrote manual input")
		}
	})
	t.Run("unknown cue is editable and opens existing creation flow", func(t *testing.T) {
		m, cmd, _ := discoveryModel(t)
		m, _ = m.Update(cmd())
		m, r := startDiscoveryRecording(m)
		m = transcript(m, r, 1, "Machine Learning Research thought, study neural networks.")
		m, _ = m.Update(voice.RecordingEnded{Recording: r})
		if m.subjectID != nil || m.voice.query.Value() != "Machine Learning Research" || m.input.Value() != "Study neural networks." {
			t.Fatal("unknown cue not retained")
		}
		m, _ = m.Update(key(tea.KeyTab))
		m, create := m.Update(key(tea.KeyEnter))
		request := create().(VoiceSubjectRequest)
		if request.Query != "Machine Learning Research" {
			t.Fatalf("create query=%q", request.Query)
		}
		stamp := m.draftObservedAt
		m.ReturnFromVoiceSubject(request, &data.Subject{SubjectID: 9, SubjectName: request.Query})
		if m.subjectID == nil || *m.subjectID != 9 || m.draftObservedAt != stamp || m.input.Value() != "Study neural networks." {
			t.Fatal("create return changed draft")
		}
	})
	t.Run("typed drafts and contextual subjects are preserved", func(t *testing.T) {
		m, cmd, _ := discoveryModel(t)
		m, _ = m.Update(cmd())
		m.input.SetValue("Existing draft.")
		m, r := startDiscoveryRecording(m)
		m = transcript(m, r, 1, "Fitness thought literal")
		m, _ = m.Update(voice.RecordingEnded{Recording: r})
		if m.input.Value() != "Existing draft.\nFitness thought literal" || m.subjectID != nil {
			t.Fatal("typed draft command interpreted")
		}
		m, cmd, _ = discoveryModel(t)
		m, _ = m.Update(cmd())
		id := int64(7)
		m.subjectID = &id
		cmd = m.EnableVoice(1, &voiceSubjects{}, "Go Programming")
		m, _ = m.Update(cmd())
		m, r = startDiscoveryRecording(m)
		m = transcript(m, r, 1, "Fitness Training thought hello")
		m, _ = m.Update(voice.RecordingEnded{Recording: r})
		if *m.subjectID != 7 || m.voice.query.Value() != "Go Programming" || m.input.Value() != "Hello" {
			t.Fatal("contextual subject overwritten")
		}
	})
	t.Run("empty attempts can retry and prefix-only text is retained", func(t *testing.T) {
		m, cmd, _ := discoveryModel(t)
		m, _ = m.Update(cmd())
		m, r := startDiscoveryRecording(m)
		m, _ = m.Update(voice.RecordingEnded{Recording: r, Failed: true})
		m, r = startDiscoveryRecording(m)
		m = transcript(m, r, 1, "Fitness thought")
		m, _ = m.Update(voice.RecordingEnded{Recording: r})
		if m.subjectID == nil || *m.subjectID != 8 || m.input.Value() != "Fitness thought" {
			t.Fatal("retry or empty-body protection failed")
		}
	})
	t.Run("retired candidate reply cannot mutate another draft", func(t *testing.T) {
		m, cmd, _ := discoveryModel(t)
		reply := cmd()
		m.OpenVoice(2, &voiceSubjects{})
		m, _ = m.Update(reply)
		if len(m.voice.items) != 0 || m.voice.query.Value() != "" {
			t.Fatal("old reply changed new draft")
		}
	})
	t.Run("unchanged cues do not rerank and rejected revisions cannot recommend", func(t *testing.T) {
		m, _, subjects := discoveryModel(t)
		ranker := &countedRecommender{}
		m.ConfigureVoiceSubjectDiscovery(subjectdiscovery.NewService(subjects, discoveryUsage{}, ranker))
		m, _ = m.Update(m.loadVoiceSubjects(subjects)())
		m, r := startDiscoveryRecording(m)
		m = transcript(m, r, 1, "Go programming practice thought first")
		m = transcript(m, r, 2, "Go programming practice thought first second")
		m = transcript(m, r, 2, "Fitness thought rejected duplicate")
		stale := r
		stale.SessionID = "old"
		m = transcript(m, stale, 3, "Fitness thought stale")
		m = transcript(m, r, 3, "Fitness thought \x1b invalid")
		if ranker.calls != 1 || m.subjectID == nil || *m.subjectID != 7 {
			t.Fatalf("ranking calls=%d subject=%v want one ranking and Go", ranker.calls, m.subjectID)
		}
	})
	t.Run("optional read failures and diagnostics preserve privacy", func(t *testing.T) {
		m, _, subjects := discoveryModel(t)
		subjects.aliasErr = errors.New("PRIVATE-DATABASE-DETAIL")
		var logs bytes.Buffer
		logger, err := logging.New(&logs, "test", "info")
		if err != nil {
			t.Fatal(err)
		}
		m.logger = logger
		m, _ = m.Update(m.loadVoiceSubjects(subjects)())
		m, r := startDiscoveryRecording(m)
		m = transcript(m, r, 1, "Go Programming thought PRIVATE-TRANSCRIPT")
		if m.subjectID == nil || *m.subjectID != 7 {
			t.Fatal("alias failure disabled canonical matching")
		}
		if strings.Contains(logs.String(), "PRIVATE-") || strings.Contains(logs.String(), "Go Programming") {
			t.Fatalf("private content in logs: %s", logs.String())
		}
		m.Reset()
		if subjects.ctx.Err() == nil {
			t.Fatal("closed draft did not cancel candidate owner")
		}
	})
	t.Run("cleanup and save retain the original opening timestamp", func(t *testing.T) {
		m, _, subjects := discoveryModel(t)
		openedAt := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
		m.now = func() time.Time { return openedAt }
		m.OpenVoice(2, subjects)
		m, _ = m.Update(m.loadVoiceSubjects(subjects)())
		m, _ = m.Update(VoiceAction{Action: "start", DraftID: 2})
		r := m.VoiceState().TranscriptAuthority.Recording()
		m = transcript(m, r, 1, "Programming in Go thought, étudier les interfaces.")
		m.now = func() time.Time { return openedAt.Add(time.Hour) }
		m, _ = m.Update(voice.RecordingEnded{Recording: r, Failed: true})
		if m.input.Value() != "Étudier les interfaces." {
			t.Fatalf("got %q want capitalized accepted body", m.input.Value())
		}
		m, save := m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
		result := save().(Result)
		if result.err != nil || !result.item.ObservedAt.Equal(openedAt) {
			t.Fatalf("save=%+v,%v want opening time %v", result.item, result.err, openedAt)
		}
	})

	t.Run("candidate results require current session request and component identity", func(t *testing.T) {
		for _, identity := range []string{"session", "request", "component"} {
			t.Run(identity, func(t *testing.T) {
				m, cmd, _ := discoveryModel(t)
				reply := cmd().(VoiceSubjectsLoaded)
				switch identity {
				case "session":
					reply.sessionID = "retired"
				case "request":
					reply.revision++
				case "component":
					reply.owner = new(int)
				}
				m, r := startDiscoveryRecording(m)
				m = transcript(m, r, 1, "Programming in Go thought hello")
				m, _ = m.Update(reply)
				if m.subjectID != nil || m.voice.suggestion.loaded {
					t.Fatal("stale candidate result was accepted")
				}
			})
		}
	})

	t.Run("a queued candidate reply stays retired after a failed save", func(t *testing.T) {
		m, cmd, _ := discoveryModel(t)
		reply := cmd()
		m.input.SetValue("Retained draft")
		m.service = failingService{err: errors.New("save failure")}
		m, save := m.Update(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
		m, _ = m.Update(save())
		m, _ = m.Update(reply)
		if len(m.voice.items) != 0 || m.input.Value() != "Retained draft" {
			t.Fatal("canceled candidate reply changed failed-save draft")
		}
	})

}

type countedRecommender struct{ calls int }

func (r *countedRecommender) Rank(ctx context.Context, cue string, candidates []subjectdiscovery.Candidate) ([]subjectdiscovery.Match, error) {
	r.calls++
	return (subjectdiscovery.NameRecommender{}).Rank(ctx, cue, candidates)
}
