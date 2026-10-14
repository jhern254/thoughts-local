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

type modelSetupStub struct {
	status  ModelSetupStatus
	install func(context.Context, func(modelassets.DownloadProgress)) error
}

func (setup *modelSetupStub) Description() modelassets.ModelDescription {
	return modelassets.ModelDescription{DisplayName: "Moonshine Small Streaming English", ExpectedSizeBytes: 142300974}
}
func (*modelSetupStub) Directory() string { return "/home/test/Thoughts/models" }
func (setup *modelSetupStub) LoadInstalledModel(context.Context) (ModelSetupStatus, error) {
	return setup.status, nil
}
func (setup *modelSetupStub) InstallAndEnableModel(ctx context.Context, report func(modelassets.DownloadProgress)) error {
	return setup.install(ctx, report)
}
func configuredModelsTestModel(setup ModelSetup) Model {
	model := newRootTestModel()
	model.ConfigureModelSetup(setup)
	return model
}
func deliverModelsCheck(model Model) Model {
	commands := model.Init()().(tea.BatchMsg)
	model, _ = rootUpdate(model, commands[1]())
	return model
}
func TestModel_ModelSetup(t *testing.T) {
	t.Run("closing Options restores F8 draft creation from Events", func(t *testing.T) {
		model := deliverModelsCheck(configuredModelsTestModel(&modelSetupStub{status: ModelSetupStatus{Ready: true}}))
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
		model, _ = rootUpdate(model, tea.KeyPressMsg{Code: tea.KeyF8})
		if !strings.Contains(model.View().Content, "Create thought") {
			t.Fatal("F8 did not open the thought draft")
		}
	})
	t.Run("mouse activates Keep writing at its rendered row", func(t *testing.T) {
		model := deliverModelsCheck(configuredModelsTestModel(&modelSetupStub{status: ModelSetupStatus{CanDownload: true}}))
		model, _ = rootUpdate(model, tea.WindowSizeMsg{Width: 100, Height: 40})
		model.models.busy = true
		model.models.status = "Downloading speech model…"
		model.models.progress = modelassets.DownloadProgress{DownloadedBytes: 50, TotalBytes: 100}
		model.models.focus = 0
		model.models.cancel = func() { t.Fatal("Keep writing cancelled the download") }

		for rowIndex, row := range strings.Split(ansi.Strip(model.View().Content), "\n") {
			if column := strings.Index(row, "Keep writing"); column >= 0 {
				model, _ = rootUpdate(model, tea.MouseClickMsg{X: ansi.StringWidth(row[:column]) + 2, Y: rowIndex, Button: tea.MouseLeft})
				if model.models.open || !model.models.busy {
					t.Fatal("Keep writing did not hide the active download")
				}
				return
			}
		}
		t.Fatal("missing rendered Keep writing action")
	})
	t.Run("missing model overlays Events and Not now restores the exact view", func(t *testing.T) {
		model := configuredModelsTestModel(&modelSetupStub{status: ModelSetupStatus{CanDownload: true}})
		before := model.View().Content
		model = deliverModelsCheck(model)
		content := ansi.Strip(model.View().Content)
		for _, text := range []string{"Events", "Models", "Download and enable", "Not now"} {
			if !strings.Contains(content, text) {
				t.Fatalf("missing %q: %s", text, content)
			}
		}
		model, _ = rootUpdate(model, enterKey())
		if model.models.open || model.View().Content != before {
			t.Fatal("Not now changed underlying view or focus")
		}
		model, _ = rootUpdate(model, modelSetupChecked{status: ModelSetupStatus{CanDownload: true}})
		if model.models.open {
			t.Fatal("reoffered in the same session")
		}
		nextLaunch := deliverModelsCheck(configuredModelsTestModel(&modelSetupStub{status: ModelSetupStatus{CanDownload: true}}))
		if !nextLaunch.models.open {
			t.Fatal("did not offer on later launch")
		}
	})
	t.Run("installed model enables speech and late session initialization cannot disable it", func(t *testing.T) {
		model := deliverModelsCheck(configuredModelsTestModel(&modelSetupStub{status: ModelSetupStatus{Ready: true}}))
		model, _ = rootUpdate(model, thoughts.BrowserVoiceSessionMsg{SessionID: "session", SpeechRecognitionAvailable: false})
		if model.models.open || !model.VoiceState().SpeechRecognitionAvailable {
			t.Fatal("ready state lost")
		}
	})
	t.Run("Options has Visual and Models and reopens setup", func(t *testing.T) {
		model := deliverModelsCheck(configuredModelsTestModel(&modelSetupStub{status: ModelSetupStatus{CanDownload: true}}))
		model, _ = rootUpdate(model, enterKey())
		model, _ = rootUpdate(model, BrowserOptionsEnabledMsg{})
		model, _ = rootUpdate(model, BrowserOptionsMsg{Action: "open"})
		if !strings.Contains(model.View().Content, "Models") || !strings.Contains(model.View().Content, "Visual") {
			t.Fatal("missing Options entries")
		}
		model, _ = rootUpdate(model, tea.KeyPressMsg{Code: tea.KeyDown})
		model, _ = rootUpdate(model, enterKey())
		if !model.models.open {
			t.Fatal("Models did not reopen popup")
		}
	})
	t.Run("narrow popup keeps actions within terminal bounds", func(t *testing.T) {
		model := deliverModelsCheck(configuredModelsTestModel(&modelSetupStub{status: ModelSetupStatus{CanDownload: true}}))
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
		setup := &modelSetupStub{status: ModelSetupStatus{CanDownload: true}, install: func(ctx context.Context, report func(modelassets.DownloadProgress)) error {
			report(modelassets.DownloadProgress{DownloadedBytes: 50, TotalBytes: 100})
			close(started)
			<-ctx.Done()
			<-release
			return ctx.Err()
		}}
		model := deliverModelsCheck(configuredModelsTestModel(setup))
		model, _ = rootUpdate(model, tea.KeyPressMsg{Code: tea.KeyUp})
		model, command := rootUpdate(model, enterKey())
		batch := command().(tea.BatchMsg)
		finished := make(chan tea.Msg, 1)
		go func() { finished <- batch[0]() }()
		<-started
		model, _ = rootUpdate(model, batch[1]())
		if model.models.progress.DownloadedBytes != 50 {
			t.Fatal("missing real progress")
		}
		model, _ = rootUpdate(model, enterKey()) // Keep writing.
		if model.models.open || !model.models.busy {
			t.Fatal("hiding cancelled download")
		}
		model.models.open = true
		model, _ = rootUpdate(model, tea.KeyPressMsg{Code: tea.KeyDown})
		model, _ = rootUpdate(model, enterKey())
		if !model.models.busy || !model.models.cancelling {
			t.Fatal("cancel did not await owned work")
		}
		close(release)
		model, _ = rootUpdate(model, <-finished)
		if model.models.busy || model.models.ready {
			t.Fatal("cancelled operation became ready")
		}
		model, _ = rootUpdate(model, modelSetupFinished{attempt: model.models.attempt - 1, err: nil})
		if model.models.ready {
			t.Fatal("stale completion enabled speech")
		}
	})
	t.Run("raw errors never become popup text", func(t *testing.T) {
		model := deliverModelsCheck(configuredModelsTestModel(&modelSetupStub{status: ModelSetupStatus{CanDownload: true}}))
		model.models.busy = true
		model, _ = rootUpdate(model, modelSetupFinished{attempt: model.models.attempt, err: errors.New("private file and content")})
		if strings.Contains(model.View().Content, "private file") {
			t.Fatal("raw error rendered")
		}
	})
}

func TestModelSetup_SessionShutdown(t *testing.T) {
	t.Run("disconnect cancels and joins download and progress commands", func(t *testing.T) {
		started := make(chan struct{})
		cancelSeen := make(chan struct{})
		release := make(chan struct{})
		setup := &modelSetupStub{status: ModelSetupStatus{CanDownload: true}, install: func(ctx context.Context, _ func(modelassets.DownloadProgress)) error {
			close(started)
			<-ctx.Done()
			close(cancelSeen)
			<-release
			return ctx.Err()
		}}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		session := NewSession(ctx, cancel, func(ctx context.Context) tea.Model {
			model := configuredModelsTestModel(setup)
			model.ctx = ctx
			return model
		})
		initCommands := session.Init()().(tea.BatchMsg)
		session.Update(initCommands[1]())
		session.Update(tea.KeyPressMsg{Code: tea.KeyUp})
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
		if session.model.(Model).models.ready {
			t.Fatal("shutdown published readiness")
		}
	})
}
