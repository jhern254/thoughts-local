package browserterm

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/jhern254/go-thoughts/internal/appearance"
	"github.com/jhern254/go-thoughts/internal/data"
)

const maxAppearanceRequestBytes = appearance.MaxImageBytes + (64 << 10)
const maxAppearanceSettingsBytes = 1024

type appearanceResponse struct {
	Background     bool `json:"background"`
	Darkness       int  `json:"darkness"`
	CleanupWarning bool `json:"cleanupWarning,omitempty"`
}

func writeAppearance(w http.ResponseWriter, settings data.Appearance, warning bool) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(appearanceResponse{
		Background:     settings.BackgroundAsset != "",
		Darkness:       settings.Darkness,
		CleanupWarning: warning,
	})
}

func appearanceError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "Could not update or load the background."
	var tooLarge *http.MaxBytesError
	switch {
	case errors.Is(err, appearance.ErrImage):
		status = http.StatusBadRequest
		message = "Choose a valid, non-animated JPEG or PNG within the image limits."
	case errors.Is(err, appearance.ErrDarkness):
		status = http.StatusBadRequest
		message = "Background darkness must be between 0 and 95."
	case errors.Is(err, appearance.ErrBusy):
		status = http.StatusConflict
		message = "An appearance change is already in progress."
	case errors.As(err, &tooLarge):
		status = http.StatusRequestEntityTooLarge
		message = "Image exceeds the upload limit."
	}
	http.Error(w, message, status)
}

func (s *server) serveAppearance(w http.ResponseWriter, r *http.Request) {
	var handler http.HandlerFunc
	var allowed string
	switch r.URL.Path {
	case "/appearance":
		allowed = "GET"
		if r.Method == http.MethodGet {
			handler = s.loadAppearance
		}
	case "/appearance/background":
		allowed = "GET, POST, DELETE"
		switch r.Method {
		case http.MethodGet:
			handler = s.serveAppearanceBackground
		case http.MethodPost:
			handler = s.importAppearanceBackground
		case http.MethodDelete:
			handler = s.removeAppearanceBackground
		}
	case "/appearance/settings":
		allowed = "PUT"
		if r.Method == http.MethodPut {
			handler = s.updateAppearanceSettings
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
	if s.appearance == nil {
		http.Error(w, "Backgrounds require a persistent application database.", http.StatusServiceUnavailable)
		return
	}
	handler(w, r)
}

func (s *server) loadAppearance(w http.ResponseWriter, r *http.Request) {
	settings, err := s.appearance.Load(r.Context())
	if err != nil {
		appearanceError(w, err)
		return
	}
	writeAppearance(w, settings, false)
}

func (s *server) serveAppearanceBackground(w http.ResponseWriter, r *http.Request) {
	file, contentType, err := s.appearance.OpenBackground(r.Context())
	if errors.Is(err, os.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		appearanceError(w, err)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		appearanceError(w, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	http.ServeContent(w, r, "", info.ModTime(), file)
}

func (s *server) removeAppearanceBackground(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAppearanceSettingsBytes)
	if _, err := io.Copy(io.Discard, r.Body); err != nil {
		appearanceError(w, err)
		return
	}
	settings, warning, err := s.appearance.Remove(r.Context())
	if err != nil {
		appearanceError(w, err)
		return
	}
	writeAppearance(w, settings, warning)
}

func (s *server) updateAppearanceSettings(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAppearanceSettingsBytes)
	var input struct {
		Darkness *int `json:"darkness"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		http.Error(w, "Invalid appearance settings.", http.StatusBadRequest)
		return
	}
	if input.Darkness == nil || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "Invalid appearance settings.", http.StatusBadRequest)
		return
	}
	settings, err := s.appearance.SetDarkness(r.Context(), *input.Darkness)
	if err != nil {
		appearanceError(w, err)
		return
	}
	writeAppearance(w, settings, false)
}

func (s *server) importAppearanceBackground(w http.ResponseWriter, r *http.Request) {
	if !s.importMu.TryLock() {
		appearanceError(w, appearance.ErrBusy)
		return
	}
	defer s.importMu.Unlock()
	r.Body = http.MaxBytesReader(w, r.Body, maxAppearanceRequestBytes)
	// Keep bounded input in memory rather than creating multipart temporary files.
	if err := r.ParseMultipartForm(maxAppearanceRequestBytes); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			appearanceError(w, err)
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
		appearanceError(w, appearance.ErrDarkness)
		return
	}
	file, err := r.MultipartForm.File["image"][0].Open()
	if err != nil {
		appearanceError(w, err)
		return
	}
	defer file.Close()
	settings, warning, err := s.appearance.Import(r.Context(), file, darkness)
	if err != nil {
		appearanceError(w, err)
		return
	}
	writeAppearance(w, settings, warning)
}
