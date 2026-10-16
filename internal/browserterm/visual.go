package browserterm

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/visual"
)

const maxVisualRequestBytes = visual.MaxImageBytes + (64 << 10)
const maxVisualSettingsBytes = 1024

type visualResponse struct {
	// Identity lets settings-only updates reuse decoded pixels. It is an opaque
	// application asset name, not a path or an alternate asset-serving route.
	BackgroundID   string                 `json:"backgroundID,omitempty"`
	Framing        data.BackgroundFraming `json:"framing"`
	Background     bool                   `json:"background"`
	Darkness       int                    `json:"darkness"`
	SpeedPercent   int                    `json:"speedPercent"`
	UserPaused     bool                   `json:"userPaused"`
	Kind           string                 `json:"kind"`
	CleanupWarning bool                   `json:"cleanupWarning,omitempty"`
}

func writeVisual(w http.ResponseWriter, settings data.Visual, warning bool) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(visualResponse{
		BackgroundID:   settings.BackgroundAsset,
		Framing:        settings.Framing,
		Background:     settings.BackgroundAsset != "",
		Darkness:       settings.Darkness,
		CleanupWarning: warning,
		SpeedPercent:   settings.SpeedPercent, UserPaused: settings.UserPaused, Kind: backgroundKind(settings.BackgroundAsset),
	})
}

func backgroundKind(asset string) string {
	if filepath.Ext(asset) == ".mp4" {
		return "video"
	}
	return "image"
}

func visualError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "Could not update or load the background."
	var tooLarge *http.MaxBytesError
	switch {
	case errors.Is(err, visual.ErrVideo):
		status = http.StatusBadRequest
		message = "Choose a self-contained SDR H.264 MP4 or MOV within the video limits."
	case errors.Is(err, visual.ErrVideoTools):
		status = http.StatusServiceUnavailable
		message = "Install FFmpeg and ffprobe to import a video background."
	case errors.Is(err, visual.ErrPlayback):
		status = http.StatusBadRequest
		message = "Video speed must be between 25 and 100 percent."
	case errors.Is(err, visual.ErrImage):
		status = http.StatusBadRequest
		message = "Choose a valid, non-animated JPEG or PNG within the image limits."
	case errors.Is(err, visual.ErrDarkness):
		status = http.StatusBadRequest
		message = "Background darkness must be between 0 and 95."
	case errors.Is(err, visual.ErrFraming):
		status = http.StatusBadRequest
		message = "Invalid background framing."
	case errors.Is(err, visual.ErrBusy):
		status = http.StatusConflict
		message = "A visual change is already in progress."
	case errors.As(err, &tooLarge):
		status = http.StatusRequestEntityTooLarge
		message = "Background exceeds the upload limit."
	}
	http.Error(w, message, status)
}

func (s *server) serveVisual(w http.ResponseWriter, r *http.Request) {
	var handler http.HandlerFunc
	var allowed string
	switch r.URL.Path {
	case "/visual":
		allowed = "GET"
		if r.Method == http.MethodGet {
			handler = s.loadVisual
		}
	case "/visual/background":
		allowed = "GET, POST, DELETE"
		switch r.Method {
		case http.MethodGet:
			handler = s.serveVisualBackground
		case http.MethodPost:
			handler = s.importVisualBackground
		case http.MethodDelete:
			handler = s.removeVisualBackground
		}
	case "/visual/video":
		allowed = "POST"
		if r.Method == http.MethodPost {
			handler = s.importVisualVideo
		}
	case "/visual/poster":
		allowed = "GET"
		if r.Method == http.MethodGet {
			handler = s.serveVisualPoster
		}
	case "/visual/settings":
		allowed = "PUT"
		if r.Method == http.MethodPut {
			handler = s.updateVisualSettings
		}
	default:
		http.NotFound(w, r)
		return
	}
	if handler == nil {
		w.Header().Set("Allow", allowed)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Method != http.MethodGet && (len(r.Header.Values("Origin")) != 1 || r.Header.Get("Origin") != "http://"+s.authority) {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if s.visual == nil {
		http.Error(w, "Backgrounds require a persistent application database.", http.StatusServiceUnavailable)
		return
	}
	handler(w, r)
}

func (s *server) loadVisual(w http.ResponseWriter, r *http.Request) {
	settings, err := s.visual.Load(r.Context())
	if err != nil {
		visualError(w, err)
		return
	}
	writeVisual(w, settings, false)
}

func (s *server) serveVisualBackground(w http.ResponseWriter, r *http.Request) {
	file, contentType, err := s.visual.OpenBackground(r.Context())
	serveVisualFile(w, r, file, contentType, err)
}

func (s *server) serveVisualPoster(w http.ResponseWriter, r *http.Request) {
	file, contentType, err := s.visual.OpenPoster(r.Context())
	serveVisualFile(w, r, file, contentType, err)
}

func serveVisualFile(w http.ResponseWriter, r *http.Request, file *os.File, contentType string, err error) {
	if errors.Is(err, os.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		visualError(w, err)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		visualError(w, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	http.ServeContent(w, r, "", info.ModTime(), file)
}

func (s *server) removeVisualBackground(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxVisualSettingsBytes)
	if _, err := io.Copy(io.Discard, r.Body); err != nil {
		visualError(w, err)
		return
	}
	settings, warning, err := s.visual.Remove(r.Context())
	if err != nil {
		visualError(w, err)
		return
	}
	writeVisual(w, settings, warning)
}

func (s *server) updateVisualSettings(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxVisualSettingsBytes)
	var input struct {
		Darkness     *int                   `json:"darkness"`
		Framing      data.BackgroundFraming `json:"framing"`
		SpeedPercent *int                   `json:"speedPercent"`
		UserPaused   bool                   `json:"userPaused"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		http.Error(w, "Invalid visual settings.", http.StatusBadRequest)
		return
	}
	if input.Darkness == nil || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "Invalid visual settings.", http.StatusBadRequest)
		return
	}
	speedPercent := 100
	if input.SpeedPercent != nil {
		speedPercent = *input.SpeedPercent
	}
	settings, err := s.visual.UpdateSettings(r.Context(), *input.Darkness, input.Framing, speedPercent, input.UserPaused)
	if err != nil {
		visualError(w, err)
		return
	}
	writeVisual(w, settings, false)
}

func (s *server) importVisualBackground(w http.ResponseWriter, r *http.Request) {
	if !s.importMu.TryLock() {
		visualError(w, visual.ErrBusy)
		return
	}
	defer s.importMu.Unlock()
	r.Body = http.MaxBytesReader(w, r.Body, maxVisualRequestBytes)
	// Keep bounded input in memory rather than creating multipart temporary files.
	if err := r.ParseMultipartForm(maxVisualRequestBytes); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			visualError(w, err)
		} else {
			http.Error(w, "Invalid image upload.", http.StatusBadRequest)
		}
		return
	}
	defer r.MultipartForm.RemoveAll()
	if len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["image"]) != 1 || len(r.MultipartForm.Value) != 2 || len(r.MultipartForm.Value["darkness"]) != 1 || len(r.MultipartForm.Value["framing"]) != 1 {
		http.Error(w, "Choose one image and a darkness value.", http.StatusBadRequest)
		return
	}
	darkness, err := strconv.Atoi(r.MultipartForm.Value["darkness"][0])
	if err != nil {
		visualError(w, visual.ErrDarkness)
		return
	}
	var framing data.BackgroundFraming
	if json.Unmarshal([]byte(r.MultipartForm.Value["framing"][0]), &framing) != nil || !framing.Valid() {
		visualError(w, visual.ErrFraming)
		return
	}
	file, err := r.MultipartForm.File["image"][0].Open()
	if err != nil {
		visualError(w, err)
		return
	}
	defer file.Close()
	settings, warning, err := s.visual.Import(r.Context(), file, darkness, framing)
	if err != nil {
		visualError(w, err)
		return
	}
	writeVisual(w, settings, warning)
}

// Video bytes stream directly into the native private staging directory. Only
// bounded settings travel in the header; filenames/MIME are never storage paths
// or codec evidence. This avoids a second 128 MiB multipart buffer or temp copy.
func (s *server) importVisualVideo(w http.ResponseWriter, r *http.Request) {
	if !s.importMu.TryLock() {
		visualError(w, visual.ErrBusy)
		return
	}
	defer s.importMu.Unlock()
	var settings struct {
		Darkness     int                    `json:"darkness"`
		Framing      data.BackgroundFraming `json:"framing"`
		SpeedPercent int                    `json:"speedPercent"`
		UserPaused   bool                   `json:"userPaused"`
	}
	headers := r.Header.Values("X-Background-Settings")
	if len(headers) != 1 || len(headers[0]) > maxVisualSettingsBytes {
		http.Error(w, "Invalid visual settings.", http.StatusBadRequest)
		return
	}
	decoder := json.NewDecoder(strings.NewReader(headers[0]))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&settings) != nil || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "Invalid visual settings.", http.StatusBadRequest)
		return
	}
	// Request cancellation alone does not interrupt a blocked HTTP body Read.
	// Bound staging on the connection as well as subprocess work, and honor an
	// earlier owner deadline. Real net/http writers support this controller;
	// recorder-only handler tests do not have a connection deadline to set.
	deadline := time.Now().Add(visual.MaxVideoProcessingTime)
	if ownerDeadline, ok := r.Context().Deadline(); ok && ownerDeadline.Before(deadline) {
		deadline = ownerDeadline
	}
	if err := http.NewResponseController(w).SetReadDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
		visualError(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, visual.MaxVideoBytes)
	selected, warning, err := s.visual.ImportVideo(r.Context(), r.Body, settings.Darkness, settings.Framing, settings.SpeedPercent, settings.UserPaused)
	if err != nil {
		visualError(w, err)
		return
	}
	writeVisual(w, selected, warning)
}
