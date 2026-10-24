package browserterm

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"runtime"
	"syscall"
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
	operationLimit := visual.MaxVideoProcessingTime
	if r.URL.Path == "/visual/video/frame" {
		operationLimit = visual.MaxVideoFrameTime
	}
	if r.Method == http.MethodPost && r.URL.Path == "/visual/video/source" {
		operationLimit += visual.MaxVideoTransferTime
	}
	ctx, cancel := context.WithTimeout(r.Context(), operationLimit)
	ownerDone := make(chan struct{})
	stopOwner := context.AfterFunc(owner, func() { defer close(ownerDone); cancel() })
	controller := http.NewResponseController(w)
	deadline := time.Now().Add(operationLimit)
	if err := controller.SetReadDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
		if !stopOwner() {
			<-ownerDone
		}
		cancel()
		videoError(w, err)
		return nil, nil, nil, false
	}
	if err := controller.SetWriteDeadline(deadline.Add(writeTimeout)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		if !stopOwner() {
			<-ownerDone
		}
		cancel()
		videoError(w, err)
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
	// Serialize this editor before admitting processor work. In particular, a
	// canceled still extraction must join before a subsequent loop preparation.
	select {
	case editor.gate <- struct{}{}:
	case <-ctx.Done():
		finishContext()
		return nil, nil, nil, false
	}
	if heavy && !s.importMu.TryLock() {
		<-editor.gate
		finishContext()
		videoError(w, visual.ErrBusy)
		return nil, nil, nil, false
	}
	finish := func() {
		finishContext()
		if heavy {
			s.importMu.Unlock()
		}
		<-editor.gate
	}
	if ctx.Err() != nil {
		finish()
		return nil, nil, nil, false
	}
	if editor.filesOwner != "" && editor.filesOwner != headers[0] {
		if err := editor.discardFiles(); err != nil {
			finish()
			videoError(w, err)
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
	r.Body = http.MaxBytesReader(w, r.Body, visual.MaxVideoSourceBytes)
	if r.ContentLength > visual.MaxVideoSourceBytes {
		videoSourceError(w, visual.ErrVideoSourceSize)
		return
	}
	// A new source POST replaces the browser File selection. Retire the old
	// staged input before receiving another movie, so replacement never needs
	// space for two originals. Accepted derivative bytes remain independently owned.
	if editor.source != nil {
		if err := editor.source.Close(); err != nil {
			videoSourceError(w, err)
			return
		}
		editor.source = nil
		editor.sourceID = ""
	}
	reader := &videoTransferReader{ctx: ctx, input: r.Body, controller: http.NewResponseController(w), deadline: time.Now().Add(visual.MaxVideoTransferTime)}
	source, err := visual.StageVideo(ctx, reader)
	if err != nil {
		videoSourceError(w, err)
		return
	}
	if err := ctx.Err(); err != nil {
		source.Close()
		return
	}
	if err := editor.discardFiles(); err != nil {
		source.Close()
		videoSourceError(w, err)
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
		videoError(w, err)
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
		SourceID     string              `json:"sourceID"`
		VideoQuality visual.VideoQuality `json:"videoQuality"`
		Trim         visual.VideoTrim    `json:"trim"`
	}
	if !readVideoEditJSON(w, r, &input) {
		return
	}
	if input.VideoQuality == "" {
		input.VideoQuality = visual.VideoQualityEfficient
	}
	if !input.VideoQuality.Valid() {
		videoError(w, visual.ErrVideoQuality)
		return
	}
	if editor.source == nil || editor.sourceID != input.SourceID {
		http.Error(w, "Video source unavailable.", http.StatusConflict)
		return
	}
	if editor.accepted != nil && editor.accepted.Info().Trim == input.Trim && editor.accepted.Info().VideoQuality == input.VideoQuality {
		writeVideoPreview(w, editor.acceptedID, editor.accepted)
		return
	}
	if editor.pending != nil && editor.pending.Info().Trim == input.Trim && editor.pending.Info().VideoQuality == input.VideoQuality {
		writeVideoPreview(w, editor.pendingID, editor.pending)
		return
	}
	prepared, err := editor.source.Prepare(ctx, input.Trim, input.VideoQuality)
	if err != nil {
		videoError(w, err)
		return
	}
	if err := ctx.Err(); err != nil {
		prepared.Close()
		return
	}
	if editor.pending != nil {
		if err := editor.pending.Close(); err != nil {
			prepared.Close()
			videoError(w, err)
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
		videoError(w, err)
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
		videoError(w, err)
		return
	}
	// A committed selection remains successful when temporary cleanup fails.
	warning = editor.discardFiles() != nil || warning
	writeVisual(w, settings, warning)
}

// Reset the idle timeout only when reading the next bounded portion. The total
// transfer deadline remains fixed, including for a peer that trickles bytes.
type videoTransferReader struct {
	ctx        context.Context
	input      io.Reader
	controller *http.ResponseController
	deadline   time.Time
}

func (reader *videoTransferReader) Read(body []byte) (int, error) {
	deadline := time.Now().Add(visual.MaxVideoTransferIdleTime)
	if reader.deadline.Before(deadline) {
		deadline = reader.deadline
	}
	if err := reader.controller.SetReadDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
		return 0, err
	}
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.input.Read(body)
}

// Video-only feedback keeps transfer failures useful without exposing OS text
// or changing the image importer's presentation contract.
func videoSourceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, visual.ErrVideoProtected):
		w.Header().Set("X-Video-Rejection", "sourceProtected")
	case errors.Is(err, visual.ErrVideoTracks):
		w.Header().Set("X-Video-Rejection", "sourceTracks")
	case errors.Is(err, visual.ErrVideoContainer):
		w.Header().Set("X-Video-Rejection", "sourceContainer")
	case errors.Is(err, visual.ErrVideoCodec):
		w.Header().Set("X-Video-Rejection", "sourceCodec")
	case errors.Is(err, visual.ErrVideoColor):
		w.Header().Set("X-Video-Rejection", "sourceColor")
	case errors.Is(err, visual.ErrVideoDimensions):
		w.Header().Set("X-Video-Rejection", "sourceDimensions")
	case errors.Is(err, visual.ErrVideoFrameRate):
		w.Header().Set("X-Video-Rejection", "sourceRate")
	case errors.Is(err, visual.ErrVideoDuration):
		w.Header().Set("X-Video-Rejection", "sourceDuration")
	}
	videoError(w, err)
}

func videoError(w http.ResponseWriter, err error) {
	var networkError net.Error
	diskFull := errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EDQUOT)
	if runtime.GOOS == "windows" {
		// Win32 reports ERROR_DISK_FULL or ERROR_HANDLE_DISK_FULL for file writes.
		diskFull = diskFull || errors.Is(err, syscall.Errno(112)) || errors.Is(err, syscall.Errno(39))
	}
	switch {
	case diskFull:
		http.Error(w, "Not enough temporary disk space for the video source.", http.StatusInsufficientStorage)
	case errors.Is(err, visual.ErrVideoSourceSize):
		http.Error(w, "Video source exceeds 8 GiB.", http.StatusRequestEntityTooLarge)
	case errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()):
		http.Error(w, "Video operation timed out. Retry with a smaller source or shorter cut.", http.StatusRequestTimeout)
	default:
		visualError(w, err)
	}
}

// Source frames have no public lookup route: every extraction requires the live
// editor capability and the exact staged source, plus bounded JSON and time.
func (s *server) previewVisualVideoFrame(w http.ResponseWriter, r *http.Request) {
	editor, ctx, finish, ok := s.beginVideoEdit(w, r, true)
	if !ok {
		return
	}
	defer finish()
	var input struct {
		SourceID   string `json:"sourceID"`
		PositionMS int64  `json:"positionMS"`
	}
	if !readVideoEditJSON(w, r, &input) {
		return
	}
	if editor.source == nil || input.SourceID == "" || editor.sourceID != input.SourceID {
		http.NotFound(w, r)
		return
	}
	body, err := editor.source.PreviewFrame(ctx, input.PositionMS)
	if err != nil {
		videoError(w, err)
		return
	}
	if ctx.Err() != nil {
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}
