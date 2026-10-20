package browserterm

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/visual"
)

// beginVideoEdit binds HTTP work to both request cancellation and the exact
// Visual opening that issued it. Cancellation also interrupts blocked socket I/O;
// finishing joins the deadline callbacks before returning the ResponseWriter.
func (s *server) beginVideoEdit(w http.ResponseWriter, r *http.Request, heavy bool) (*videoEditor, context.Context, func(), bool) {
	s.mu.Lock()
	editor := s.videoEditor
	s.mu.Unlock()
	headers := r.Header.Values("X-Visual-Editor")
	if editor == nil || len(headers) != 1 {
		http.Error(w, "Visual editor unavailable.", http.StatusForbidden)
		return nil, nil, nil, false
	}
	owner, valid := editor.context(headers[0])
	if !valid {
		http.Error(w, "Visual editor unavailable.", http.StatusForbidden)
		return nil, nil, nil, false
	}
	ctx, cancel := context.WithTimeout(r.Context(), visual.MaxVideoProcessingTime)
	ownerDone := make(chan struct{})
	stopOwner := context.AfterFunc(owner, func() { defer close(ownerDone); cancel() })
	controller := http.NewResponseController(w)
	deadline := time.Now().Add(visual.MaxVideoProcessingTime)
	if err := controller.SetReadDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
		if !stopOwner() {
			<-ownerDone
		}
		cancel()
		visualError(w, err)
		return nil, nil, nil, false
	}
	if err := controller.SetWriteDeadline(deadline.Add(writeTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		if !stopOwner() {
			<-ownerDone
		}
		cancel()
		visualError(w, err)
		return nil, nil, nil, false
	}
	abortDone := make(chan struct{})
	stopAbort := context.AfterFunc(ctx, func() {
		defer close(abortDone)
		_ = controller.SetReadDeadline(time.Now())
		// An upload can still receive a fixed error after its read is interrupted.
		// A stalled retrieval must instead stop writing before its files are retired.
		if r.Method == http.MethodGet {
			_ = controller.SetWriteDeadline(time.Now())
		} else {
			_ = controller.SetWriteDeadline(time.Now().Add(writeTimeout))
		}
	})
	finishContext := func() {
		if !stopOwner() {
			<-ownerDone
		}
		if !stopAbort() {
			<-abortDone
		}
		cancel()
	}
	if heavy && !s.importMu.TryLock() {
		finishContext()
		visualError(w, visual.ErrBusy)
		return nil, nil, nil, false
	}
	releaseHeavy := func() {
		if heavy {
			s.importMu.Unlock()
		}
	}
	select {
	case editor.gate <- struct{}{}:
	case <-ctx.Done():
		finishContext()
		releaseHeavy()
		return nil, nil, nil, false
	}
	finish := func() { finishContext(); <-editor.gate; releaseHeavy() }
	if ctx.Err() != nil {
		finish()
		return nil, nil, nil, false
	}
	if editor.filesOwner != "" && editor.filesOwner != headers[0] {
		if err := editor.discardFiles(); err != nil {
			finish()
			visualError(w, err)
			return nil, nil, nil, false
		}
	}
	editor.filesOwner = headers[0]
	return editor, ctx, finish, true
}

func readVideoEditJSON(w http.ResponseWriter, r *http.Request, value any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxVisualSettingsBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(value) != nil || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "Invalid video controls.", http.StatusBadRequest)
		return false
	}
	return true
}
func writeVideoPreview(w http.ResponseWriter, id string, prepared *visual.PreparedVideo) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		PreviewID string `json:"previewID"`
		visual.PreparedVideoInfo
	}{id, prepared.Info()})
}
func (s *server) stageVisualVideoSource(w http.ResponseWriter, r *http.Request) {
	editor, ctx, finish, ok := s.beginVideoEdit(w, r, true)
	if !ok {
		return
	}
	defer finish()
	r.Body = http.MaxBytesReader(w, r.Body, visual.MaxVideoBytes)
	source, err := visual.StageVideo(ctx, r.Body)
	if err != nil {
		visualError(w, err)
		return
	}
	if err := ctx.Err(); err != nil {
		source.Close()
		return
	}
	if err := editor.discardFiles(); err != nil {
		source.Close()
		visualError(w, err)
		return
	}
	editor.filesOwner = r.Header.Get("X-Visual-Editor")
	editor.source = source
	editor.sourceID = rand.Text()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		SourceID string `json:"sourceID"`
		visual.VideoSourceInfo
	}{editor.sourceID, source.Info()})
}
func (s *server) discardVisualVideoSource(w http.ResponseWriter, r *http.Request) {
	editor, _, finish, ok := s.beginVideoEdit(w, r, false)
	if !ok {
		return
	}
	defer finish()
	if err := editor.discardFiles(); err != nil {
		visualError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *server) prepareVisualVideoLoop(w http.ResponseWriter, r *http.Request) {
	editor, ctx, finish, ok := s.beginVideoEdit(w, r, true)
	if !ok {
		return
	}
	defer finish()
	var input struct {
		SourceID string           `json:"sourceID"`
		Trim     visual.VideoTrim `json:"trim"`
	}
	if !readVideoEditJSON(w, r, &input) {
		return
	}
	if editor.source == nil || editor.sourceID != input.SourceID {
		http.Error(w, "Video source unavailable.", http.StatusConflict)
		return
	}
	if editor.accepted != nil && editor.accepted.Info().Trim == input.Trim {
		writeVideoPreview(w, editor.acceptedID, editor.accepted)
		return
	}
	if editor.pending != nil && editor.pending.Info().Trim == input.Trim {
		writeVideoPreview(w, editor.pendingID, editor.pending)
		return
	}
	prepared, err := editor.source.Prepare(ctx, input.Trim, visual.VideoQualityEfficient)
	if err != nil {
		visualError(w, err)
		return
	}
	if err := ctx.Err(); err != nil {
		prepared.Close()
		return
	}
	if editor.pending != nil {
		if err := editor.pending.Close(); err != nil {
			prepared.Close()
			visualError(w, err)
			return
		}
	}
	editor.pending = prepared
	editor.pendingID = rand.Text()
	writeVideoPreview(w, editor.pendingID, prepared)
}
func (s *server) acceptVisualVideoLoop(w http.ResponseWriter, r *http.Request) {
	editor, _, finish, ok := s.beginVideoEdit(w, r, false)
	if !ok {
		return
	}
	defer finish()
	var input struct {
		PreviewID string `json:"previewID"`
	}
	if !readVideoEditJSON(w, r, &input) {
		return
	}
	if editor.accepted != nil && input.PreviewID == editor.acceptedID {
		writeVideoPreview(w, editor.acceptedID, editor.accepted)
		return
	}
	if editor.pending == nil || input.PreviewID != editor.pendingID {
		http.NotFound(w, r)
		return
	}
	warning := false
	if editor.accepted != nil {
		warning = editor.accepted.Close() != nil
	}
	editor.accepted = editor.pending
	editor.acceptedID = editor.pendingID
	editor.pending = nil
	editor.pendingID = ""
	// The new preview is accepted even if retiring its predecessor left a file.
	if warning {
		w.Header().Set("X-Visual-Cleanup-Warning", "true")
	}
	writeVideoPreview(w, editor.acceptedID, editor.accepted)
}
func (s *server) discardVisualVideoPreview(w http.ResponseWriter, r *http.Request) {
	editor, _, finish, ok := s.beginVideoEdit(w, r, false)
	if !ok {
		return
	}
	defer finish()
	var input struct {
		PreviewID string `json:"previewID"`
	}
	if !readVideoEditJSON(w, r, &input) {
		return
	}
	if editor.pending == nil || editor.pendingID != input.PreviewID {
		http.NotFound(w, r)
		return
	}
	if err := editor.pending.Close(); err != nil {
		visualError(w, err)
		return
	}
	editor.pending = nil
	editor.pendingID = ""
	w.WriteHeader(http.StatusNoContent)
}
func (s *server) serveVisualVideoPreview(w http.ResponseWriter, r *http.Request) {
	editor, ctx, finish, ok := s.beginVideoEdit(w, r, false)
	if !ok {
		return
	}
	defer finish()
	part := r.URL.Query().Get("part")
	if part != "video" && part != "poster" {
		http.NotFound(w, r)
		return
	}
	id := r.URL.Query().Get("id")
	prepared := editor.pending
	if id == editor.acceptedID && id != "" {
		prepared = editor.accepted
	} else if id != editor.pendingID || id == "" {
		http.NotFound(w, r)
		return
	}
	if prepared == nil {
		http.NotFound(w, r)
		return
	}
	file, err := prepared.Open(ctx, part == "poster")
	kind := "video/mp4"
	if part == "poster" {
		kind = "image/jpeg"
	}
	w.Header().Set("Cache-Control", "no-store")
	serveVisualFile(w, r, file, kind, err)
}
func (s *server) applyVisualVideoLoop(w http.ResponseWriter, r *http.Request) {
	editor, ctx, finish, ok := s.beginVideoEdit(w, r, false)
	if !ok {
		return
	}
	defer finish()
	var input struct {
		PreviewID    string                 `json:"previewID"`
		Darkness     int                    `json:"darkness"`
		Framing      data.BackgroundFraming `json:"framing"`
		SpeedPercent int                    `json:"speedPercent"`
		UserPaused   bool                   `json:"userPaused"`
	}
	if !readVideoEditJSON(w, r, &input) {
		return
	}
	if editor.accepted == nil || input.PreviewID != editor.acceptedID {
		http.Error(w, "Preview the video loop before applying.", http.StatusConflict)
		return
	}
	settings, warning, err := s.visual.ApplyPreparedVideo(ctx, editor.accepted, input.Darkness, input.Framing, input.SpeedPercent, input.UserPaused)
	if err != nil {
		visualError(w, err)
		return
	}
	// A committed selection remains successful when temporary cleanup fails.
	warning = editor.discardFiles() != nil || warning
	writeVisual(w, settings, warning)
}
