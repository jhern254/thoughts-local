package browserterm

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/visual"
)

const maxVisualRequestBytes = visual.MaxImageBytes + (64 << 10)
const maxVisualSettingsBytes = 1024

type visualResponse struct {
	VideoQuality string `json:"videoQuality"`
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

	BorderWeight data.BorderWeight `json:"borderWeight"`
	HeavierText  bool              `json:"heavierText"`
	TextSize     data.TextSize     `json:"textSize"`
	HighContrast bool              `json:"highContrast"`
}

func writeVisual(w http.ResponseWriter, settings data.Visual, warning bool) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(visualResponse{
		BackgroundID:   settings.BackgroundAsset,
		VideoQuality:   settings.VideoQuality,
		Framing:        settings.Framing,
		Background:     settings.BackgroundAsset != "",
		Darkness:       settings.Darkness,
		CleanupWarning: warning,
		SpeedPercent:   settings.SpeedPercent, UserPaused: settings.UserPaused, Kind: backgroundKind(settings.BackgroundAsset),

		BorderWeight: settings.BorderWeight,
		HeavierText:  settings.HeavierText,
		TextSize:     settings.TextSize,
		HighContrast: settings.HighContrast,
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
	case errors.Is(err, visual.ErrVideoQuality):
		status = http.StatusBadRequest
		message = "Choose Efficient, High quality, or Maximum video quality."
	case errors.Is(err, visual.ErrVideo):
		status = http.StatusBadRequest
		message = "Choose a supported self-contained SDR MP4, MOV or MKV within the video limits."
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
			handler = s.applyVisualVideoLoop
		}
	case "/visual/video/frame":
		allowed = "POST"
		if r.Method == http.MethodPost {
			handler = s.previewVisualVideoFrame
		}
	case "/visual/video/preview/stop":
		allowed = "POST"
		if r.Method == http.MethodPost {
			handler = s.stopVisualVideoPreview
		}
	case "/visual/video/source":
		allowed = "POST, DELETE"
		switch r.Method {
		case http.MethodPost:
			handler = s.stageVisualVideoSource
		case http.MethodDelete:
			handler = s.discardVisualVideoSource
		}
	case "/visual/video/preview":
		allowed = "GET, POST, PUT, DELETE"
		switch r.Method {
		case http.MethodGet:
			handler = s.serveVisualVideoPreview
		case http.MethodPost:
			handler = s.prepareVisualVideoLoop
		case http.MethodPut:
			handler = s.acceptVisualVideoLoop
		case http.MethodDelete:
			handler = s.discardVisualVideoPreview
		}
	case "/visual/poster":
		allowed = "GET"
		if r.Method == http.MethodGet {
			handler = s.serveVisualPoster
		}
	case "/visual/accessibility":
		allowed = "PUT"
		if r.Method == http.MethodPut {
			handler = s.updateAccessibility
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
		VideoQuality visual.VideoQuality    `json:"videoQuality"`
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
	settings, err := s.visual.UpdateSettings(r.Context(), *input.Darkness, input.Framing, speedPercent, input.UserPaused, input.VideoQuality)
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
	qualityValues, hasQuality := r.MultipartForm.Value["videoQuality"]
	expectedFields := 2
	if hasQuality {
		expectedFields++
	}
	if len(r.MultipartForm.File) != 1 || len(r.MultipartForm.File["image"]) != 1 || len(r.MultipartForm.Value) != expectedFields || len(r.MultipartForm.Value["darkness"]) != 1 || len(r.MultipartForm.Value["framing"]) != 1 || (hasQuality && len(qualityValues) != 1) {
		http.Error(w, "Choose one image and valid visual preferences.", http.StatusBadRequest)
		return
	}
	quality := visual.VideoQuality("")
	if hasQuality {
		quality = visual.VideoQuality(qualityValues[0])
		if !quality.Valid() {
			visualError(w, visual.ErrVideoQuality)
			return
		}
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
	settings, warning, err := s.visual.Import(r.Context(), file, darkness, framing, quality)
	if err != nil {
		visualError(w, err)
		return
	}
	writeVisual(w, settings, warning)
}

func (s *server) updateAccessibility(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxVisualSettingsBytes)
	var input struct {
		BorderWeight *data.BorderWeight `json:"borderWeight"`
		HeavierText  *bool              `json:"heavierText"`
		TextSize     *data.TextSize     `json:"textSize"`
		HighContrast *bool              `json:"highContrast"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil || input.BorderWeight == nil || input.HeavierText == nil || input.TextSize == nil || input.HighContrast == nil || decoder.Decode(new(any)) != io.EOF {
		http.Error(w, "Invalid accessibility settings.", http.StatusBadRequest)
		return
	}
	settings, err := s.visual.UpdateAccessibility(r.Context(), data.AccessibilityPreferences{
		BorderWeight: *input.BorderWeight,
		HeavierText:  *input.HeavierText,
		TextSize:     *input.TextSize,
		HighContrast: *input.HighContrast,
	})
	if errors.Is(err, visual.ErrAccessibility) {
		http.Error(w, "Invalid accessibility settings.", http.StatusBadRequest)
		return
	}
	if err != nil {
		visualError(w, err)
		return
	}
	writeVisual(w, settings, false)
}
