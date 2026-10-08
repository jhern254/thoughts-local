package browserterm

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/visual"
)

const maxVisualRequestBytes = visual.MaxImageBytes + (64 << 10)
const maxVisualSettingsBytes = 1024

type visualResponse struct {
	Background     bool `json:"background"`
	Darkness       int  `json:"darkness"`
	CleanupWarning bool `json:"cleanupWarning,omitempty"`
}

func writeVisual(w http.ResponseWriter, settings data.Visual, warning bool) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(visualResponse{
		Background:     settings.BackgroundAsset != "",
		Darkness:       settings.Darkness,
		CleanupWarning: warning,
	})
}

func visualError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "Could not update or load the background."
	var tooLarge *http.MaxBytesError
	switch {
	case errors.Is(err, visual.ErrImage):
		status = http.StatusBadRequest
		message = "Choose a valid, non-animated JPEG or PNG within the image limits."
	case errors.Is(err, visual.ErrDarkness):
		status = http.StatusBadRequest
		message = "Background darkness must be between 0 and 95."
	case errors.Is(err, visual.ErrBusy):
		status = http.StatusConflict
		message = "A visual change is already in progress."
	case errors.As(err, &tooLarge):
		status = http.StatusRequestEntityTooLarge
		message = "Image exceeds the upload limit."
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
		Darkness *int `json:"darkness"`
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
	settings, err := s.visual.SetDarkness(r.Context(), *input.Darkness)
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
	if len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["image"]) != 1 || len(r.MultipartForm.Value) != 1 || len(r.MultipartForm.Value["darkness"]) != 1 {
		http.Error(w, "Choose one image and a darkness value.", http.StatusBadRequest)
		return
	}
	darkness, err := strconv.Atoi(r.MultipartForm.Value["darkness"][0])
	if err != nil {
		visualError(w, visual.ErrDarkness)
		return
	}
	file, err := r.MultipartForm.File["image"][0].Open()
	if err != nil {
		visualError(w, err)
		return
	}
	defer file.Close()
	settings, warning, err := s.visual.Import(r.Context(), file, darkness)
	if err != nil {
		visualError(w, err)
		return
	}
	writeVisual(w, settings, warning)
}
