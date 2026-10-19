package browserterm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/visual"
)

func videoEditRequest(t *testing.T, s *server, editorID, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "http://127.0.0.1:7777"+path, bytes.NewReader(body))
	r.Header.Set("Origin", "http://127.0.0.1:7777")
	r.Header.Set("X-Visual-Editor", editorID)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestVideoEditor_Routes(t *testing.T) {
	t.Run("requires the current editor capability and never interprets a file path", func(t *testing.T) {
		s := &server{
			videoEditor: newVideoEditor(t.Context()),
			visual:      visual.NewService(&visualStore{}, "user", t.TempDir()),
			authority:   "127.0.0.1:7777",
		}
		id, _ := s.videoEditor.setOpen(true)
		defer s.videoEditor.Close()
		for _, path := range []string{"/visual/video/source", "/visual/video/preview", "/visual/video"} {
			w := videoEditRequest(t, s, "PRIVATE", "POST", path, []byte("PRIVATE"))
			if w.Code != 403 || strings.Contains(w.Body.String(), "PRIVATE") {
				t.Fatalf("%s: got %d %q", path, w.Code, w.Body.String())
			}
			r := httptest.NewRequest("POST", "http://127.0.0.1:7777"+path, nil)
			r.Header.Set("X-Visual-Editor", id)
			w = httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != 403 {
				t.Fatalf("%s missing origin: got %d", path, w.Code)
			}
		}
		w := videoEditRequest(t, s, id, "GET", "/visual/video/preview?id=../../PRIVATE&part=video", nil)
		if w.Code != 404 || strings.Contains(w.Body.String(), "PRIVATE") {
			t.Fatalf("path lookup: %d %q", w.Code, w.Body.String())
		}
		_, cleanup := s.videoEditor.setOpen(false)
		if err := cleanup(); err != nil {
			t.Fatal(err)
		}
		newID, _ := s.videoEditor.setOpen(true)
		if newID == id {
			t.Fatal("editor capability reused")
		}
		if w := videoEditRequest(t, s, id, "DELETE", "/visual/video/source", nil); w.Code != 403 {
			t.Fatalf("closed editor status %d", w.Code)
		}
	})
	t.Run("acknowledges exact prepared bytes and preserves accepted loop after a failed cut", func(t *testing.T) {
		for _, tool := range []string{"ffmpeg", "ffprobe"} {
			if _, err := exec.LookPath(tool); err != nil {
				t.Skip("video tools not installed")
			}
		}
		body, err := os.ReadFile("client/fixtures/visual-video.mp4")
		if err != nil {
			t.Fatal(err)
		}
		store := &visualStore{}
		s := &server{
			videoEditor: newVideoEditor(t.Context()),
			visual:      visual.NewService(store, "user", t.TempDir()),
			authority:   "127.0.0.1:7777",
		}
		id, _ := s.videoEditor.setOpen(true)
		defer s.videoEditor.Close()
		staged := videoEditRequest(t, s, id, "POST", "/visual/video/source", body)
		if staged.Code != 200 {
			t.Fatalf("stage: %d %s", staged.Code, staged.Body.String())
		}
		var source struct {
			SourceID string `json:"sourceID"`
		}
		if err := json.Unmarshal(staged.Body.Bytes(), &source); err != nil {
			t.Fatal(err)
		}
		input, _ := json.Marshal(map[string]any{"sourceID": source.SourceID, "trim": visual.VideoTrim{StartMS: 250, EndMS: 1500}})
		prepared := videoEditRequest(t, s, id, "POST", "/visual/video/preview", input)
		if prepared.Code != 200 {
			t.Fatalf("prepare: %d %s", prepared.Code, prepared.Body.String())
		}
		var preview struct {
			PreviewID string `json:"previewID"`
		}
		if err := json.Unmarshal(prepared.Body.Bytes(), &preview); err != nil {
			t.Fatal(err)
		}
		apply, _ := json.Marshal(map[string]any{"previewID": preview.PreviewID, "darkness": 70, "framing": data.DefaultBackgroundFraming(), "speedPercent": 25, "userPaused": false})
		if w := videoEditRequest(t, s, id, "POST", "/visual/video", apply); w.Code != 409 {
			t.Fatalf("unacknowledged apply: %d", w.Code)
		}
		want := videoEditRequest(t, s, id, "GET", "/visual/video/preview?id="+preview.PreviewID+"&part=video", nil)
		if want.Code != 200 {
			t.Fatalf("preview retrieval: %d", want.Code)
		}
		accept, _ := json.Marshal(preview)
		if w := videoEditRequest(t, s, id, "PUT", "/visual/video/preview", accept); w.Code != 200 {
			t.Fatalf("accept: %d %s", w.Code, w.Body.String())
		}
		next, _ := json.Marshal(map[string]any{"sourceID": source.SourceID, "trim": visual.VideoTrim{EndMS: 1000}})
		pending := videoEditRequest(t, s, id, "POST", "/visual/video/preview", next)
		if pending.Code != 200 {
			t.Fatalf("replacement preview: %d %s", pending.Code, pending.Body.String())
		}
		var replacement struct {
			PreviewID string `json:"previewID"`
		}
		if err := json.Unmarshal(pending.Body.Bytes(), &replacement); err != nil {
			t.Fatal(err)
		}
		if w := videoEditRequest(t, s, id, "GET", "/visual/video/preview?id="+preview.PreviewID+"&part=video", nil); !bytes.Equal(w.Body.Bytes(), want.Body.Bytes()) {
			t.Fatal("pending preview replaced accepted bytes")
		}
		discard, _ := json.Marshal(replacement)
		if w := videoEditRequest(t, s, id, "DELETE", "/visual/video/preview", discard); w.Code != 204 {
			t.Fatalf("discard pending: %d", w.Code)
		}
		if w := videoEditRequest(t, s, id, "GET", "/visual/video/preview?id="+replacement.PreviewID+"&part=video", nil); w.Code != 404 {
			t.Fatalf("discarded candidate: %d", w.Code)
		}

		// Identical cuts reuse the acknowledged files; no second processor is needed.
		t.Setenv("PATH", t.TempDir())
		if w := videoEditRequest(t, s, id, "POST", "/visual/video/preview", input); w.Code != 200 || !bytes.Equal(w.Body.Bytes(), prepared.Body.Bytes()) {
			t.Fatalf("same-cut reuse: %d %s", w.Code, w.Body.String())
		}
		bad, _ := json.Marshal(map[string]any{"sourceID": source.SourceID, "trim": visual.VideoTrim{StartMS: 2000, EndMS: 1000}})
		if w := videoEditRequest(t, s, id, "POST", "/visual/video/preview", bad); w.Code != 400 {
			t.Fatalf("invalid range: %d", w.Code)
		}
		store.fail = true
		if w := videoEditRequest(t, s, id, "POST", "/visual/video", apply); w.Code != 500 || strings.Contains(w.Body.String(), "PRIVATE") {
			t.Fatalf("failed save: %d %s", w.Code, w.Body.String())
		}
		store.fail = false
		if w := videoEditRequest(t, s, id, "POST", "/visual/video", apply); w.Code != 200 {
			t.Fatalf("apply: %d %s", w.Code, w.Body.String())
		}
		file, _, err := s.visual.OpenBackground(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		got, err := io.ReadAll(file)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want.Body.Bytes()) {
			t.Fatal("saved bytes differ from the acknowledged loop")
		}
		if s.videoEditor.source != nil || s.videoEditor.accepted != nil {
			t.Fatal("Apply retained temporary source or loop")
		}
		if w := videoEditRequest(t, s, id, "GET", "/visual/video/preview?id="+preview.PreviewID+"&part=video", nil); w.Code != 404 {
			t.Fatalf("retired preview: %d", w.Code)
		}
	})
	t.Run("closing an editor cancels admitted work and joins before cleanup", func(t *testing.T) {
		editor := newVideoEditor(t.Context())
		id, _ := editor.setOpen(true)
		ctx, ok := editor.context(id)
		if !ok {
			t.Fatal("owner missing")
		}
		editor.gate <- struct{}{}
		_, cleanup := editor.setOpen(false)
		if ctx.Err() != context.Canceled {
			t.Fatal("close did not cancel work")
		}
		done := make(chan error, 1)
		go func() { done <- cleanup() }()
		select {
		case <-done:
			t.Fatal("cleanup did not join admitted operation")
		default:
		}
		<-editor.gate
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if err := editor.Close(); err != nil {
			t.Fatal(err)
		}
	})
}
