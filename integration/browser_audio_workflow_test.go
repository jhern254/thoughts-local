//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/coder/websocket"
	"github.com/jhern254/go-thoughts/internal/application"
	"github.com/jhern254/go-thoughts/internal/browserterm"
	"github.com/jhern254/go-thoughts/internal/logging"
	"github.com/jhern254/go-thoughts/internal/tui"
	"github.com/jhern254/go-thoughts/internal/tui/thoughts"
	"github.com/jhern254/go-thoughts/internal/voice"
)

func TestBrowserAudioWorkflow_SQLite(t *testing.T) {
	t.Run("audio and unsaved transcript never write SQLite files or operational logs", func(t *testing.T) {
		database, dsn := openMigratedSQLite(t)
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()
		runtime, err := application.Open(ctx, dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer runtime.Close()
		// data_version detects commits from the application's separate connection
		// across every table, rather than checking only the thoughts count.
		observer, err := database.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer observer.Close()
		var initialVersion int
		if err := observer.QueryRowContext(ctx, "PRAGMA data_version").Scan(&initialVersion); err != nil {
			t.Fatal(err)
		}
		temporaryDirectory := t.TempDir()
		t.Setenv("TMPDIR", temporaryDirectory)
		t.Setenv("TMP", temporaryDirectory)
		t.Setenv("TEMP", temporaryDirectory)
		var diagnostics bytes.Buffer
		logger, err := logging.New(&diagnostics, "audio-test", "debug")
		if err != nil {
			t.Fatal(err)
		}
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		views := make(chan string, 100)
		consumerDone := make(chan struct{})
		consumer := func(ctx context.Context, recording voice.RecordingKey, samples []int16, publish func(voice.TranscriptUpdate) bool) error {
			defer close(consumerDone)
			publish(voice.TranscriptUpdate{Recording: recording, Revision: 1, Text: "synthetic transcript"})
			<-ctx.Done()
			return errors.New("PRIVATE-CONSUMER-ERROR")
		}
		serverDone := make(chan error, 1)
		go func() {
			serverDone <- browserterm.ServeWithAudioConsumer(ctx, listener, func(session context.Context) tea.Model {
				return browserObservedModel{Model: tui.NewModel(session, runtime.LocalUser(), runtime.Subjects(), runtime.Thoughts(), runtime.Metrics(), runtime.Events(), runtime.TimelineView(), logger), views: views}
			}, logger, consumer)
		}()
		serverJoined := false
		defer func() {
			cancel()
			if !serverJoined {
				if err := <-serverDone; err != nil {
					t.Error(err)
				}
			}
		}()
		address := "http://" + listener.Addr().String()
		terminal, _, err := websocket.Dial(ctx, "ws://"+listener.Addr().String()+"/ws", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {address}}})
		if err != nil {
			t.Fatal(err)
		}
		defer terminal.CloseNow()
		writeControl := func(frame string) {
			t.Helper()
			if err := terminal.Write(ctx, websocket.MessageBinary, []byte(frame)); err != nil {
				t.Fatal(err)
			}
		}
		type grant struct {
			thoughts.VoiceState
			AudioCapability string `json:"audioCapability"`
		}
		waitGrant := func(matches func(grant) bool) grant {
			t.Helper()
			for {
				_, frame, err := terminal.Read(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if len(frame) == 0 || frame[0] != 'v' {
					continue
				}
				var recordingGrant grant
				if err := json.Unmarshal(frame[1:], &recordingGrant); err != nil {
					t.Fatal(err)
				}
				if matches(recordingGrant) {
					return recordingGrant
				}
			}
		}
		waitView := func(text string) {
			t.Helper()
			for {
				select {
				case rendered := <-views:
					if strings.Contains(ansi.Strip(rendered), text) {
						return
					}
				case <-ctx.Done():
					t.Fatal("expected editor state was not reached")
				}
			}
		}
		writeControl(`2{"cols":100,"rows":35}`)
		waitGrant(func(recording grant) bool { return recording.CanOpenThoughtDraft })
		writeControl("0t")
		waitGrant(func(recording grant) bool { return recording.DraftID != 0 })
		writeControl("ptyped text")
		waitView("typed text")
		writeControl(`v{"action":"start","draftID":1}`)
		recordingGrant := waitGrant(func(recording grant) bool { return recording.AudioCapability != "" })
		audioConnection, _, err := websocket.Dial(ctx, "ws://"+listener.Addr().String()+"/voice/audio", &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {address}}})
		if err != nil {
			t.Fatal(err)
		}
		defer audioConnection.CloseNow()
		// The integration exercises the documented wire contract independently
		// of the private protocol decoder used by production.
		handshake := make([]byte, 76)
		copy(handshake, []byte{'T', 'A', 1, 1})
		sessionBytes, err := hex.DecodeString(recordingGrant.SessionID)
		if err != nil {
			t.Fatal(err)
		}
		copy(handshake[4:20], sessionBytes)
		binary.LittleEndian.PutUint64(handshake[20:28], recordingGrant.DraftID)
		binary.LittleEndian.PutUint64(handshake[28:36], recordingGrant.RecordingID)
		capabilityBytes, err := hex.DecodeString(recordingGrant.AudioCapability)
		if err != nil {
			t.Fatal(err)
		}
		copy(handshake[36:68], capabilityBytes)
		binary.LittleEndian.PutUint32(handshake[68:72], 16000)
		handshake[72], handshake[73] = 1, 1
		if err := audioConnection.Write(ctx, websocket.MessageBinary, handshake); err != nil {
			t.Fatal(err)
		}
		_, status, err := audioConnection.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(status, []byte{'T', 'A', 1, 3, 0}) {
			t.Fatalf("authorization status got %v, want Ready", status)
		}
		pcmFrame := make([]byte, 12+3200)
		copy(pcmFrame, []byte{'T', 'A', 1, 2})
		binary.LittleEndian.PutUint32(pcmFrame[8:12], 1600)
		copy(pcmFrame[12:], "PRIVATE-PCM")
		if err := audioConnection.Write(ctx, websocket.MessageBinary, pcmFrame); err != nil {
			t.Fatal(err)
		}
		waitView("synthetic transcript")
		writeControl(`v{"action":"stop","draftID":1,"recordingID":1}`)
		waitGrant(func(recording grant) bool { return recording.RecordingStatus == "idle" })
		<-consumerDone
		var afterAudioVersion int
		if err := observer.QueryRowContext(ctx, "PRAGMA data_version").Scan(&afterAudioVersion); err != nil {
			t.Fatal(err)
		}
		if got, want := afterAudioVersion, initialVersion; got != want {
			t.Fatalf("SQLite version after audio got %d, want unchanged %d", got, want)
		}
		files, err := os.ReadDir(temporaryDirectory)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := len(files), 0; got != want {
			t.Fatalf("audio temporary files got %d, want %d", got, want)
		}
		writeControl("0\x13")
		waitView("Thought 1 •")
		var savedText string
		if err := observer.QueryRowContext(ctx, "SELECT thought FROM thoughts WHERE thought_id=1").Scan(&savedText); err != nil {
			t.Fatal(err)
		}
		if got, want := savedText, "typed text\nsynthetic transcript"; got != want {
			t.Fatalf("explicitly saved thought got %q, want %q", got, want)
		}
		cancel()
		// The server must finish before reading a logger buffer it can write.
		if err := <-serverDone; err != nil {
			t.Fatal(err)
		}
		serverJoined = true
		for _, private := range []string{"PRIVATE-PCM", "PRIVATE-CONSUMER-ERROR", "synthetic transcript", "typed text", recordingGrant.AudioCapability} {
			if strings.Contains(diagnostics.String(), private) {
				t.Error("operational logs contained private recording content")
			}
		}
	})
}
