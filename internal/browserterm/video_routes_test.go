package browserterm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/visual"
)

func videoEditRequest(t *testing.T, s *server, editorID, method, path string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "http://127.0.0.1:7777"+path, bytes.NewReader(body))
	r.Header.Set("Origin", "http://127.0.0.1:7777")
	r.Header.Set("X-Visual-Editor", editorID)
	if method == http.MethodPost && path == "/visual/video/preview" {
		if _, valid := s.videoEditor.context(editorID); valid {
			// Direct HTTP tests stand in for the Go Options bridge admitting a render.
			s.videoEditor.mu.Lock()
			operation := s.videoEditor.previewRequest + 1
			s.videoEditor.mu.Unlock()
			s.videoEditor.setPreviewOperation(operation, true)
			r.Header.Set("X-Visual-Operation", strconv.FormatUint(operation, 10))
		}
	}
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
		for _, path := range []string{"/visual/video/source", "/visual/video/preview", "/visual/video", "/visual/video/frame"} {
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
		frameInput, _ := json.Marshal(map[string]any{"sourceID": source.SourceID, "positionMS": 250})
		frame := videoEditRequest(t, s, id, "POST", "/visual/video/frame", frameInput)
		if frame.Code != 200 || frame.Header().Get("Content-Type") != "image/jpeg" {
			t.Fatalf("frame: %d %s", frame.Code, frame.Body.String())
		}
		if _, err := jpeg.Decode(bytes.NewReader(frame.Body.Bytes())); err != nil {
			t.Fatal(err)
		}
		for _, invalid := range []string{`{"sourceID":"../../PRIVATE","positionMS":0}`, `{"sourceID":"` + source.SourceID + `","positionMS":-1}`, `{"sourceID":"` + source.SourceID + `","positionMS":9223372036854775807}`} {
			response := videoEditRequest(t, s, id, "POST", "/visual/video/frame", []byte(invalid))
			if response.Code == 200 || strings.Contains(response.Body.String(), "PRIVATE") {
				t.Fatalf("unsafe frame: %d %s", response.Code, response.Body.String())
			}
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

		store.loadErr = data.ErrVisualSchemaMismatch
		blocked := videoEditRequest(t, s, id, "POST", "/visual/video/preview", next)
		if blocked.Code != 500 || blocked.Header().Get("X-Visual-Error") != "schema" {
			t.Fatalf("blocked preparation: got %d %q, want schema failure", blocked.Code, blocked.Header().Get("X-Visual-Error"))
		}
		if s.videoEditor.accepted == nil || s.videoEditor.acceptedID != preview.PreviewID {
			t.Fatal("settings failure discarded accepted loop")
		}
		store.loadErr = nil

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

func TestVideoSource_Admission(t *testing.T) {
	t.Run("source rejections expose only fixed reasons", func(t *testing.T) {
		for _, scenario := range []struct {
			err    error
			reason string
		}{
			{visual.ErrVideoProtected, "sourceProtected"},
			{visual.ErrVideoTracks, "sourceTracks"},
			{visual.ErrVideoContainer, "sourceContainer"},
			{visual.ErrVideoCodec, "sourceCodec"},
			{visual.ErrVideoColor, "sourceColor"},
			{visual.ErrVideoDimensions, "sourceDimensions"},
			{visual.ErrVideoFrameRate, "sourceRate"},
			{visual.ErrVideoDuration, "sourceDuration"},
		} {
			t.Run(scenario.reason, func(t *testing.T) {
				response := httptest.NewRecorder()
				videoSourceError(response, errors.Join(scenario.err, errors.New("PRIVATE decoder path")))
				if response.Code != http.StatusBadRequest || response.Header().Get("X-Video-Rejection") != scenario.reason {
					t.Fatalf("got %d %q, want 400 %q", response.Code, response.Header().Get("X-Video-Rejection"), scenario.reason)
				}
				if strings.Contains(response.Body.String(), "PRIVATE") || strings.Contains(response.Header().Get("X-Video-Rejection"), "PRIVATE") {
					t.Fatal("rejection exposed decoder text")
				}
			})
		}
	})
	t.Run("transfer does not consume the preparation deadline", func(t *testing.T) {
		s := &server{videoEditor: newVideoEditor(t.Context())}
		id, _ := s.videoEditor.setOpen(true)
		defer s.videoEditor.Close()
		for _, path := range []string{"/visual/video/source", "/visual/video/preview"} {
			r := httptest.NewRequest("POST", path, nil)
			r.Header.Set("X-Visual-Editor", id)
			if path == "/visual/video/preview" {
				s.videoEditor.setPreviewOperation(1, true)
				r.Header.Set("X-Visual-Operation", "1")
			}
			_, ctx, finish, ok := s.beginVideoEdit(httptest.NewRecorder(), r, false)
			if !ok {
				t.Fatal("admission failed")
			}
			deadline, _ := ctx.Deadline()
			remaining := time.Until(deadline)
			finish()
			want := visual.MaxVideoProcessingTime
			if path == "/visual/video/source" {
				want += visual.MaxVideoTransferTime
			}
			if remaining > want || remaining < want-time.Second {
				t.Fatalf("%s deadline=%v, want approximately %v", path, remaining, want)
			}
		}
	})
	t.Run("rejects oversized Content-Length before reading", func(t *testing.T) {
		s := &server{videoEditor: newVideoEditor(t.Context()), authority: "127.0.0.1:7777", visual: visual.NewService(&visualStore{}, "user", t.TempDir())}
		id, _ := s.videoEditor.setOpen(true)
		defer s.videoEditor.Close()
		r := httptest.NewRequest("POST", "http://127.0.0.1:7777/visual/video/source", nil)
		r.Header.Set("Origin", "http://127.0.0.1:7777")
		r.Header.Set("X-Visual-Editor", id)
		r.ContentLength = visual.MaxVideoSourceBytes + 1
		r.Body = unreadableVideoBody{t: t}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 413 || strings.Contains(w.Body.String(), "PRIVATE") {
			t.Fatalf("response=%d %q, want fixed 413", w.Code, w.Body.String())
		}
	})
	t.Run("disk failures return fixed feedback without OS paths", func(t *testing.T) {
		failures := []error{syscall.ENOSPC, syscall.EDQUOT}
		if runtime.GOOS == "windows" {
			failures = append(failures, syscall.Errno(112), syscall.Errno(39))
		}
		for _, failure := range failures {
			w := httptest.NewRecorder()
			videoError(w, &os.PathError{Op: "write", Path: "PRIVATE", Err: failure})
			if w.Code != 507 || !strings.Contains(w.Body.String(), "temporary disk space") || strings.Contains(w.Body.String(), "PRIVATE") {
				t.Fatalf("disk failure %v: response=%d %q", failure, w.Code, w.Body.String())
			}
		}
	})
}

type unreadableVideoBody struct{ t *testing.T }

func (body unreadableVideoBody) Read([]byte) (int, error) {
	body.t.Fatal("oversized source body was read")
	return 0, io.EOF
}
func (unreadableVideoBody) Close() error { return nil }

func TestVideoSource_ReadDeadline(t *testing.T) {
	t.Run("a stalled transfer keeps its finite total deadline", func(t *testing.T) {
		response := &videoDeadlineResponse{ResponseRecorder: httptest.NewRecorder()}
		total := time.Now().Add(-time.Second)
		reader := videoTransferReader{ctx: t.Context(), input: deadlineVideoInput{response}, controller: http.NewResponseController(response), deadline: total}
		_, err := reader.Read(make([]byte, 32))
		if !errors.Is(err, os.ErrDeadlineExceeded) || !response.deadline.Equal(total) {
			t.Fatalf("read=%v deadline=%v, want expired total %v", err, response.deadline, total)
		}
		w := httptest.NewRecorder()
		videoError(w, err)
		if w.Code != 408 {
			t.Fatalf("status=%d, want 408", w.Code)
		}
	})
	t.Run("cancellation wins even if the idle deadline was just reset", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		response := &videoDeadlineResponse{ResponseRecorder: httptest.NewRecorder()}
		reader := videoTransferReader{ctx: ctx, input: unreadableVideoBody{t: t}, controller: http.NewResponseController(response), deadline: time.Now().Add(visual.MaxVideoTransferTime)}
		_, err := reader.Read(make([]byte, 32))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("read=%v, want canceled", err)
		}
		if remaining := time.Until(response.deadline); remaining > visual.MaxVideoTransferIdleTime || remaining < visual.MaxVideoTransferIdleTime-time.Second {
			t.Fatalf("idle deadline=%v, want approximately 30s", remaining)
		}
	})
}

type videoDeadlineResponse struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (response *videoDeadlineResponse) SetReadDeadline(deadline time.Time) error {
	response.deadline = deadline
	return nil
}

type deadlineVideoInput struct{ response *videoDeadlineResponse }

func (input deadlineVideoInput) Read([]byte) (int, error) {
	if input.response.deadline.Before(time.Now()) {
		return 0, os.ErrDeadlineExceeded
	}
	return 0, io.EOF
}

func TestVideoSource_Replacement(t *testing.T) {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("requires video tools")
	}
	sources := t.TempDir()
	t.Setenv("TMPDIR", sources)
	s := &server{videoEditor: newVideoEditor(t.Context()), authority: "127.0.0.1:7777", visual: visual.NewService(&visualStore{}, "user", t.TempDir())}
	id, _ := s.videoEditor.setOpen(true)
	defer s.videoEditor.Close()
	fixture, err := os.ReadFile("client/fixtures/visual-video.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if w := videoEditRequest(t, s, id, "POST", "/visual/video/source", fixture); w.Code != 200 {
		t.Fatalf("initial stage=%d", w.Code)
	}
	r := httptest.NewRequest("POST", "http://127.0.0.1:7777/visual/video/source", nil)
	r.Header.Set("Origin", "http://127.0.0.1:7777")
	r.Header.Set("X-Visual-Editor", id)
	r.Body = io.NopCloser(replacementVideoBody{t: t, directory: sources})
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatalf("invalid replacement=%d, want 400", w.Code)
	}
	files, err := os.ReadDir(sources)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 || s.videoEditor.source != nil {
		t.Fatalf("replacement failure retained %d inputs", len(files))
	}
}

type replacementVideoBody struct {
	t         *testing.T
	directory string
}

func (body replacementVideoBody) Read([]byte) (int, error) {
	files, err := os.ReadDir(body.directory)
	if err != nil {
		body.t.Fatal(err)
	}
	if len(files) != 1 {
		body.t.Fatalf("staging owns %d source directories, want only the replacement", len(files))
	}
	return 0, io.EOF
}

func TestVideoSource_BrowserTimeline(t *testing.T) {
	tool, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("requires video tools")
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "offset.mp4")
	command := exec.CommandContext(t.Context(), tool, "-v", "error", "-f", "lavfi", "-i", "color=size=32x24:rate=24:duration=2", "-c:v", "libx264", "-threads", "1", "-pix_fmt", "yuv420p", "-output_ts_offset", "5", "-movflags", "+faststart", path)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, output)
	}
	staging := filepath.Join(directory, "staging")
	if err := os.Mkdir(staging, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", staging)
	s := &server{videoEditor: newVideoEditor(t.Context()), authority: "127.0.0.1:7777", visual: visual.NewService(&visualStore{}, "user", filepath.Join(directory, "assets"))}
	id, _ := s.videoEditor.setOpen(true)
	defer s.videoEditor.Close()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	response := videoEditRequest(t, s, id, "POST", "/visual/video/source", body)
	if response.Code != http.StatusOK {
		t.Fatalf("timeline response=%d %q", response.Code, response.Body.String())
	}
	files, err := os.ReadDir(staging)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || s.videoEditor.source == nil {
		t.Fatal("nonzero timestamp prevented normal source staging")
	}
	if got := s.videoEditor.source.Info().DurationMS; got != 2000 {
		t.Fatalf("source duration = %d, want 2000 independent of timestamp origin", got)
	}
	if err := s.videoEditor.Close(); err != nil {
		t.Fatal(err)
	}
	files, err = os.ReadDir(staging)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatal("closing editor retained staged source")
	}
}

func TestVideoEditor_PreviewCancellation(t *testing.T) {
	t.Run("render cancellation keeps the editor open and rejects stale operations", func(t *testing.T) {
		editor := newVideoEditor(t.Context())
		defer editor.Close()
		id, _ := editor.setOpen(true)
		editor.setPreviewOperation(5, true)
		rendering, valid := editor.previewContext(id, 5)
		if !valid || rendering.Err() != nil {
			t.Fatal("rendering permission unavailable")
		}
		editor.setPreviewOperation(6, false)
		if !errors.Is(rendering.Err(), context.Canceled) {
			t.Fatal("Stop did not cancel native rendering")
		}
		owner, valid := editor.context(id)
		if !valid || owner.Err() != nil {
			t.Fatal("Stop closed the source editor")
		}
		editor.setPreviewOperation(7, true)
		if _, valid := editor.previewContext(id, 5); valid {
			t.Fatal("retired rendering operation was admitted")
		}
		current, valid := editor.previewContext(id, 7)
		if !valid || current.Err() != nil {
			t.Fatal("new rendering operation inherited cancellation")
		}
	})
}

func TestVideoEditor_StopRendering(t *testing.T) {
	t.Run("joins canceled work while retaining source and acknowledged loop", func(t *testing.T) {
		if _, err := exec.LookPath("ffmpeg"); err != nil {
			t.Skip("requires video tools")
		}
		s := &server{videoEditor: newVideoEditor(t.Context()), authority: "127.0.0.1:7777", visual: visual.NewService(&visualStore{}, "user", t.TempDir())}
		id, _ := s.videoEditor.setOpen(true)
		defer s.videoEditor.Close()
		fixture, err := os.ReadFile("client/fixtures/visual-video.mp4")
		if err != nil {
			t.Fatal(err)
		}
		if got := videoEditRequest(t, s, id, "POST", "/visual/video/source", fixture); got.Code != 200 {
			t.Fatalf("stage=%d", got.Code)
		}
		sourceID := s.videoEditor.sourceID
		controls := func(start, end int64) []byte {
			body, err := json.Marshal(map[string]any{"sourceID": sourceID, "trim": visual.VideoTrim{StartMS: start, EndMS: end}})
			if err != nil {
				t.Fatal(err)
			}
			return body
		}
		if got := videoEditRequest(t, s, id, "POST", "/visual/video/preview", controls(0, 1000)); got.Code != 200 {
			t.Fatalf("prepare=%d", got.Code)
		}
		acceptedID := s.videoEditor.pendingID
		if got := videoEditRequest(t, s, id, "PUT", "/visual/video/preview", []byte(`{"previewID":"`+acceptedID+`"}`)); got.Code != 200 {
			t.Fatalf("accept=%d", got.Code)
		}
		selected := videoEditRequest(t, s, id, "GET", "/visual/video/preview?id="+acceptedID+"&part=video", nil)
		if got := videoEditRequest(t, s, id, "POST", "/visual/video/preview", controls(250, 1750)); got.Code != 200 {
			t.Fatalf("candidate=%d", got.Code)
		}
		operationID := s.videoEditor.previewRequest
		request := httptest.NewRequest("POST", "http://127.0.0.1:7777/visual/video/preview", nil)
		request.Header.Set("X-Visual-Editor", id)
		request.Header.Set("X-Visual-Operation", strconv.FormatUint(operationID, 10))
		_, processing, finish, ok := s.beginVideoEdit(httptest.NewRecorder(), request, true)
		if !ok {
			t.Fatal("render was not admitted")
		}
		joined := make(chan struct{})
		go func() { <-processing.Done(); close(joined); finish() }()
		s.videoEditor.setPreviewOperation(operationID+1, false)
		body, _ := json.Marshal(map[string]any{"sourceID": sourceID, "operationID": operationID})
		stopped := videoEditRequest(t, s, id, "POST", "/visual/video/preview/stop", body)
		if stopped.Code != 204 {
			t.Fatalf("stop=%d, want 204", stopped.Code)
		}
		select {
		case <-joined:
		default:
			t.Fatal("Stop replied before processing joined")
		}
		if s.videoEditor.sourceID != sourceID || s.videoEditor.pending != nil || s.videoEditor.acceptedID != acceptedID {
			t.Fatal("Stop discarded source/accepted loop or retained candidate")
		}
		late := httptest.NewRequest("POST", "http://127.0.0.1:7777/visual/video/preview", bytes.NewReader(controls(250, 1750)))
		late.Header.Set("Origin", "http://127.0.0.1:7777")
		late.Header.Set("X-Visual-Editor", id)
		late.Header.Set("X-Visual-Operation", strconv.FormatUint(operationID, 10))
		refused := httptest.NewRecorder()
		s.ServeHTTP(refused, late)
		if refused.Code != http.StatusForbidden {
			t.Fatalf("late render=%d, want 403", refused.Code)
		}
		retained := videoEditRequest(t, s, id, "GET", "/visual/video/preview?id="+acceptedID+"&part=video", nil)
		if retained.Code != 200 || !bytes.Equal(retained.Body.Bytes(), selected.Body.Bytes()) {
			t.Fatal("Stop changed acknowledged bytes")
		}
		if got := videoEditRequest(t, s, id, "POST", "/visual/video/preview", controls(0, 1000)); got.Code != 200 {
			t.Fatalf("retry=%d", got.Code)
		}
		if got := videoEditRequest(t, s, id, "POST", "/visual/video/preview/stop", body); got.Code != 409 {
			t.Fatalf("stale stop=%d, want 409", got.Code)
		}
	})
}

func TestVideoEditor_StopBoundary(t *testing.T) {
	for _, scenario := range []struct {
		name, origin, host, method string
		want                       int
	}{
		{"missing Origin", "", "127.0.0.1:7777", "POST", 403},
		{"foreign Origin", "http://PRIVATE", "127.0.0.1:7777", "POST", 403},
		{"wrong Host", "http://127.0.0.1:7777", "PRIVATE", "POST", 403},
		{"unexpected method", "http://127.0.0.1:7777", "127.0.0.1:7777", "PUT", 405},
		{"invalid controls", "http://127.0.0.1:7777", "127.0.0.1:7777", "POST", 400},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			s := &server{
				authority:   "127.0.0.1:7777",
				visual:      visual.NewService(&visualStore{}, "user", t.TempDir()),
				videoEditor: newVideoEditor(t.Context()),
			}
			id, _ := s.videoEditor.setOpen(true)
			defer s.videoEditor.Close()
			request := httptest.NewRequest(scenario.method, "http://127.0.0.1:7777/visual/video/preview/stop", strings.NewReader("PRIVATE"))
			request.Host = scenario.host
			request.Header.Set("Origin", scenario.origin)
			request.Header.Set("X-Visual-Editor", id)
			response := httptest.NewRecorder()
			s.ServeHTTP(response, request)
			if response.Code != scenario.want {
				t.Fatalf("status=%d, want %d", response.Code, scenario.want)
			}
			if strings.Contains(response.Body.String(), "PRIVATE") {
				t.Fatal("private input exposed")
			}
		})
	}
}
