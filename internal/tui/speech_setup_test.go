package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jhern254/go-thoughts/internal/modelassets"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
)

type speechSetupStub struct {
	status  SpeechModelSetupStatus
	install func(context.Context, func(modelassets.DownloadProgress)) error
	repair  func(context.Context, func(modelassets.DownloadProgress)) error
}

func (setup *speechSetupStub) Description() modelassets.ModelDescription {
	return modelassets.ModelDescription{DisplayName: "Moonshine Small Streaming English", ExpectedSizeBytes: 142300974}
}
func (*speechSetupStub) Directory() string { return "/home/test/Documents/Thoughts/models" }
func (setup *speechSetupStub) LoadInstalledSpeechModel(context.Context) (SpeechModelSetupStatus, error) {
	return setup.status, nil
}
func (setup *speechSetupStub) InstallAndEnableSpeechModel(ctx context.Context, report func(modelassets.DownloadProgress)) (SpeechModelSetupStatus, error) {
	err := setup.install(ctx, report)
	if err != nil {
		return setup.status, err
	}
	return SpeechModelSetupStatus{Ready: true}, nil
}
func (setup *speechSetupStub) RepairAndEnableSpeechModel(ctx context.Context, report func(modelassets.DownloadProgress)) (SpeechModelSetupStatus, error) {
	err := setup.repair(ctx, report)
	if err != nil {
		return setup.status, err
	}
	return SpeechModelSetupStatus{Ready: true}, nil
}
func configuredSpeechSetupTestModel(setup SpeechModelSetup) Model {
	model := newRootTestModel()
	model.ConfigureSpeechModelSetup(setup)
	return model
}
func deliverSpeechSetupCheck(model Model) Model {
	commands := model.Init()().(tea.BatchMsg)
	model, _ = rootUpdate(model, commands[1]())
	return model
}
func TestModel_SpeechSetup(t *testing.T) {
	t.Run("download is selected by default and Not now explains the recommendation", func(t *testing.T) {
		model := deliverSpeechSetupCheck(configuredSpeechSetupTestModel(&speechSetupStub{status: SpeechModelSetupStatus{CanDownload: true}}))
		if !strings.Contains(ansi.Strip(model.View().Content), "Not now (recommended to download)") {
			t.Error("missing download recommendation beside Not now")
		}
		model, command := rootUpdate(model, enterKey())
		if !model.speechSetup.busy || command == nil || !model.speechSetup.open {
			t.Fatal("Enter did not start the default download action")
		}
		model.speechSetup.cancel()
	})
	t.Run("closing Options restores typed draft creation from Events", func(t *testing.T) {
		model := deliverSpeechSetupCheck(configuredSpeechSetupTestModel(&speechSetupStub{status: SpeechModelSetupStatus{Ready: true}}))
		model, _ = rootUpdate(model, thoughts.BrowserVoiceSessionMsg{SessionID: "session", SpeechRecognitionAvailable: true})
		model, _ = rootUpdate(model, BrowserOptionsEnabledMsg{})
		if !model.VoiceState().CanOpenThoughtDraft {
			t.Fatal("Events did not allow draft creation")
		}
		model, _ = rootUpdate(model, BrowserOptionsMsg{Action: "open"})
		if model.VoiceState().CanOpenThoughtDraft {
			t.Fatal("Options allowed draft creation behind the menu")
		}
		model, _ = rootUpdate(model, tea.KeyPressMsg{Code: tea.KeyEscape})
		if !model.VoiceState().CanOpenThoughtDraft {
			t.Fatal("closing Options did not restore draft creation")
		}
		model, _ = rootUpdate(model, tea.KeyPressMsg{Code: 't', Text: "t"})
		if !strings.Contains(model.View().Content, "Create thought") {
			t.Fatal("T did not open the thought draft")
		}
	})
	t.Run("mouse activates Keep writing at its rendered row", func(t *testing.T) {
		model := deliverSpeechSetupCheck(configuredSpeechSetupTestModel(&speechSetupStub{status: SpeechModelSetupStatus{CanDownload: true}}))
		model, _ = rootUpdate(model, tea.WindowSizeMsg{Width: 100, Height: 40})
		model.speechSetup.busy = true
		model.speechSetup.status = "Downloading speech model…"
		model.speechSetup.progress = modelassets.DownloadProgress{DownloadedBytes: 50, TotalBytes: 100}
		model.speechSetup.focus = 0
		model.speechSetup.cancel = func() { t.Fatal("Keep writing cancelled the download") }

		for rowIndex, row := range strings.Split(ansi.Strip(model.View().Content), "\n") {
			if column := strings.Index(row, "Keep writing"); column >= 0 {
				model, _ = rootUpdate(model, tea.MouseClickMsg{X: ansi.StringWidth(row[:column]) + 2, Y: rowIndex, Button: tea.MouseLeft})
				if model.speechSetup.open || !model.speechSetup.busy {
					t.Fatal("Keep writing did not hide the active download")
				}
				return
			}
		}
		t.Fatal("missing rendered Keep writing action")
	})
	t.Run("missing model overlays Events and Not now restores the exact view", func(t *testing.T) {
		model := configuredSpeechSetupTestModel(&speechSetupStub{status: SpeechModelSetupStatus{CanDownload: true}})
		before := model.View().Content
		model = deliverSpeechSetupCheck(model)
		content := ansi.Strip(model.View().Content)
		for _, text := range []string{"Events", "Models", "Download and enable", "Not now"} {
			if !strings.Contains(content, text) {
				t.Fatalf("missing %q: %s", text, content)
			}
		}
		model, _ = rootUpdate(model, tea.KeyPressMsg{Code: tea.KeyDown})
		model, _ = rootUpdate(model, enterKey())
		if model.speechSetup.open || model.View().Content != before {
			t.Fatal("Not now changed underlying view or focus")
		}
		model, _ = rootUpdate(model, speechSetupChecked{status: SpeechModelSetupStatus{CanDownload: true}})
		if model.speechSetup.open {
			t.Fatal("reoffered in the same session")
		}
		nextLaunch := deliverSpeechSetupCheck(configuredSpeechSetupTestModel(&speechSetupStub{status: SpeechModelSetupStatus{CanDownload: true}}))
		if !nextLaunch.speechSetup.open {
			t.Fatal("did not offer on later launch")
		}
	})
	t.Run("installed model enables speech and late session initialization cannot disable it", func(t *testing.T) {
		model := deliverSpeechSetupCheck(configuredSpeechSetupTestModel(&speechSetupStub{status: SpeechModelSetupStatus{Ready: true}}))
		model, _ = rootUpdate(model, thoughts.BrowserVoiceSessionMsg{SessionID: "session", SpeechRecognitionAvailable: false})
		if model.speechSetup.open || !model.VoiceState().SpeechRecognitionAvailable {
			t.Fatal("ready state lost")
		}
	})
	t.Run("Options has Visual and Models and reopens setup", func(t *testing.T) {
		model := deliverSpeechSetupCheck(configuredSpeechSetupTestModel(&speechSetupStub{status: SpeechModelSetupStatus{CanDownload: true}}))
		model, _ = rootUpdate(model, tea.KeyPressMsg{Code: tea.KeyDown})
		model, _ = rootUpdate(model, enterKey())
		model, _ = rootUpdate(model, BrowserOptionsEnabledMsg{})
		model, _ = rootUpdate(model, BrowserOptionsMsg{Action: "open"})
		if !strings.Contains(model.View().Content, "Models") || !strings.Contains(model.View().Content, "Visual") {
			t.Fatal("missing Options entries")
		}
		model, _ = rootUpdate(model, tea.KeyPressMsg{Code: tea.KeyDown})
		model, _ = rootUpdate(model, enterKey())
		if !model.speechSetup.open {
			t.Fatal("Models did not reopen popup")
		}
		model, command := rootUpdate(model, enterKey())
		if !model.speechSetup.busy || command == nil {
			t.Fatal("reopened Models did not select Download by default")
		}
		model.speechSetup.cancel()
	})
	t.Run("narrow popup keeps actions within terminal bounds", func(t *testing.T) {
		model := deliverSpeechSetupCheck(configuredSpeechSetupTestModel(&speechSetupStub{status: SpeechModelSetupStatus{CanDownload: true}}))
		model, _ = rootUpdate(model, tea.WindowSizeMsg{Width: 40, Height: 18})
		content := ansi.Strip(model.View().Content)
		if !strings.Contains(content, "Download and enable") || !strings.Contains(content, "Not now") {
			t.Fatal("lost narrow actions")
		}
		for _, row := range strings.Split(content, "\n") {
			if ansi.StringWidth(row) > 40 {
				t.Fatalf("row exceeds width: %q", row)
			}
		}
		if len(strings.Split(content, "\n")) > 18 {
			t.Fatal("popup exceeds height")
		}
	})
	t.Run("hidden download reports progress and cancel waits for completion before retry", func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})
		setup := &speechSetupStub{status: SpeechModelSetupStatus{CanDownload: true}, install: func(ctx context.Context, report func(modelassets.DownloadProgress)) error {
			report(modelassets.DownloadProgress{DownloadedBytes: 50, TotalBytes: 100})
			close(started)
			<-ctx.Done()
			<-release
			return ctx.Err()
		}}
		model := deliverSpeechSetupCheck(configuredSpeechSetupTestModel(setup))
		model, command := rootUpdate(model, enterKey())
		batch := command().(tea.BatchMsg)
		finished := make(chan tea.Msg, 1)
		go func() { finished <- batch[0]() }()
		<-started
		model, _ = rootUpdate(model, batch[1]())
		if model.speechSetup.progress.DownloadedBytes != 50 {
			t.Fatal("missing real progress")
		}
		model, _ = rootUpdate(model, enterKey()) // Keep writing.
		if model.speechSetup.open || !model.speechSetup.busy {
			t.Fatal("hiding cancelled download")
		}
		model.speechSetup.open = true
		model, _ = rootUpdate(model, tea.KeyPressMsg{Code: tea.KeyDown})
		model, _ = rootUpdate(model, enterKey())
		if !model.speechSetup.busy || !model.speechSetup.cancelling {
			t.Fatal("cancel did not await owned work")
		}
		close(release)
		model, _ = rootUpdate(model, <-finished)
		if model.speechSetup.busy || model.speechSetup.ready {
			t.Fatal("cancelled operation became ready")
		}
		model, _ = rootUpdate(model, speechSetupFinished{attempt: model.speechSetup.attempt - 1, err: nil})
		if model.speechSetup.ready {
			t.Fatal("stale completion enabled speech")
		}
	})
	t.Run("raw errors never become popup text", func(t *testing.T) {
		model := deliverSpeechSetupCheck(configuredSpeechSetupTestModel(&speechSetupStub{status: SpeechModelSetupStatus{CanDownload: true}}))
		model.speechSetup.busy = true
		model, _ = rootUpdate(model, speechSetupFinished{attempt: model.speechSetup.attempt, err: errors.New("private file and content")})
		if strings.Contains(model.View().Content, "private file") {
			t.Fatal("raw error rendered")
		}
	})
}

func TestSpeechSetup_SessionShutdown(t *testing.T) {
	t.Run("disconnect cancels and joins download and progress commands", func(t *testing.T) {
		started := make(chan struct{})
		cancelSeen := make(chan struct{})
		release := make(chan struct{})
		setup := &speechSetupStub{status: SpeechModelSetupStatus{CanDownload: true}, install: func(ctx context.Context, _ func(modelassets.DownloadProgress)) error {
			close(started)
			<-ctx.Done()
			close(cancelSeen)
			<-release
			return ctx.Err()
		}}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		session := NewSession(ctx, cancel, func(ctx context.Context) tea.Model {
			model := configuredSpeechSetupTestModel(setup)
			model.ctx = ctx
			return model
		})
		initCommands := session.Init()().(tea.BatchMsg)
		session.Update(initCommands[1]())
		_, command := session.Update(enterKey())
		commands := command().(tea.BatchMsg)
		downloadDone := make(chan tea.Msg, 1)
		go func() { downloadDone <- commands[0]() }()
		<-started
		progressDone := make(chan struct{})
		go func() { commands[1](); close(progressDone) }()
		stopped := make(chan struct{})
		go func() { session.Stop(); close(stopped) }()
		<-cancelSeen
		select {
		case <-stopped:
			t.Fatal("shutdown returned before download joined")
		default:
		}
		close(release)
		<-downloadDone
		<-progressDone
		<-stopped
		if session.model.(Model).speechSetup.ready {
			t.Fatal("shutdown published readiness")
		}
	})
}

func TestModel_SpeechRepair(t *testing.T) {
	t.Run("repair is explicit and selected by default", func(t *testing.T) {
		repairs := 0
		setup := &speechSetupStub{status: SpeechModelSetupStatus{CanRepair: true}, repair: func(ctx context.Context, report func(modelassets.DownloadProgress)) error {
			repairs++
			report(modelassets.DownloadProgress{DownloadedBytes: 50, TotalBytes: 100})
			return nil
		}}
		model := deliverSpeechSetupCheck(configuredSpeechSetupTestModel(setup))
		content := ansi.Strip(model.View().Content)
		if !strings.Contains(content, "Saved model files need repair.") || !strings.Contains(content, "Repair and redownload") || strings.Contains(content, "Download and enable") {
			t.Fatalf("wrong repair view: %s", content)
		}
		model, _ = rootUpdate(model, tea.KeyPressMsg{Code: tea.KeyDown})
		model, _ = rootUpdate(model, enterKey())
		if repairs != 0 || model.speechSetup.open {
			t.Fatal("Not now performed repair")
		}
		model, _ = rootUpdate(model, BrowserOptionsEnabledMsg{})
		model, _ = rootUpdate(model, BrowserOptionsMsg{Action: "open"})
		model, _ = rootUpdate(model, tea.KeyPressMsg{Code: tea.KeyDown})
		model, _ = rootUpdate(model, enterKey())
		if !model.speechSetup.open {
			t.Fatal("Models did not reopen repair")
		}
		model, command := rootUpdate(model, enterKey())
		if !model.speechSetup.busy {
			t.Fatal("default action did not start repair")
		}
		batch := command().(tea.BatchMsg)
		finished := batch[0]()
		model, _ = rootUpdate(model, finished)
		if repairs != 1 || !model.speechSetup.ready || model.speechSetup.canRepair {
			t.Fatal("repair did not become ready")
		}
		model, _ = rootUpdate(model, speechSetupProgress{attempt: model.speechSetup.attempt - 1, progress: modelassets.DownloadProgress{DownloadedBytes: 99}})
		if model.speechSetup.progress.DownloadedBytes == 99 {
			t.Fatal("stale progress accepted")
		}
	})
	t.Run("cancelled repair receives missing state without stale readiness", func(t *testing.T) {
		model := deliverSpeechSetupCheck(configuredSpeechSetupTestModel(&speechSetupStub{status: SpeechModelSetupStatus{CanRepair: true}}))
		model, _ = rootUpdate(model, enterKey())
		model.speechSetup.cancel()
		model, _ = rootUpdate(model, speechSetupFinished{attempt: model.speechSetup.attempt, status: SpeechModelSetupStatus{CanDownload: true}, err: context.Canceled})
		if model.speechSetup.ready || model.speechSetup.canRepair || !model.speechSetup.canDownload {
			t.Fatal("cancelled repair has incoherent state")
		}
	})
}
