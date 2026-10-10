package browserterm

import (
	"bytes"
	tea "charm.land/bubbletea/v2"
	"context"
	"errors"
	"github.com/jhern254/go-thoughts/internal/logging"
	"image"
	"image/jpeg"
	"image/png"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jhern254/go-thoughts/internal/data"
	"github.com/jhern254/go-thoughts/internal/visual"
)

type visualStore struct {
	settings data.Visual
	fail     bool
}

func (s *visualStore) Load(context.Context, string) (data.Visual, error) {
	return s.settings, nil
}
func (s *visualStore) Save(_ context.Context, _ string, value data.Visual) error {
	if s.fail {
		return errors.New("PRIVATE /secret/database.sqlite")
	}
	s.settings = value
	return nil
}
func visualRequest(t *testing.T, format string) (*http.Request, []byte) {
	t.Helper()
	var imageBytes bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 12, 8))
	switch format {
	case "jpeg":
		_ = jpeg.Encode(&imageBytes, img, nil)
	case "png":
		_ = png.Encode(&imageBytes, img)
	default:
		imageBytes.WriteString("PRIVATE <svg/>")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	// The extension and multipart MIME are deliberately wrong for valid files.
	part, err := writer.CreateFormFile("image", "../../PRIVATE.svg")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(imageBytes.Bytes())
	_ = writer.WriteField("darkness", "35")
	_ = writer.WriteField("framing", `{"fit":"fill","zoom":100,"positionX":5000,"positionY":5000}`)
	_ = writer.Close()
	request := httptest.NewRequest("POST", "http://127.0.0.1:7777/visual/background", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Origin", "http://127.0.0.1:7777")
	return request, imageBytes.Bytes()
}
func TestVisual_Routes(t *testing.T) {
	for _, format := range []string{"jpeg", "png"} {
		t.Run("imports "+format+" by decoded content and serves only selected asset", func(t *testing.T) {
			store := &visualStore{}
			s := &server{authority: "127.0.0.1:7777", visual: visual.NewService(store, "user", t.TempDir())}
			req, want := visualRequest(t, format)
			w := httptest.NewRecorder()
			s.ServeHTTP(w, req)
			if w.Code != 200 {
				t.Fatalf("import = %d: %s", w.Code, w.Body.String())
			}
			w = httptest.NewRecorder()
			s.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:7777/visual/background", nil))
			if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), want) {
				t.Fatalf("retrieval = %d, want original image", w.Code)
			}
			for _, path := range []string{"/visual/../PRIVATE", "/visual/background/../../PRIVATE", "/visual/" + store.settings.BackgroundAsset} {
				w = httptest.NewRecorder()
				s.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:7777"+path, nil))
				if w.Code != 404 {
					t.Fatalf("%s = %d, want 404", path, w.Code)
				}
			}
			remove := httptest.NewRequest("DELETE", "http://127.0.0.1:7777/visual/background", nil)
			remove.Header.Set("Origin", "http://127.0.0.1:7777")
			w = httptest.NewRecorder()
			s.ServeHTTP(w, remove)
			if w.Code != 200 || store.settings.BackgroundAsset != "" || store.settings.Darkness != 35 {
				t.Fatalf("remove = %d, %+v", w.Code, store.settings)
			}
		})
	}
	for _, kind := range []string{"missing origin", "foreign origin", "duplicate origin", "wrong host", "forwarded host", "wrong method", "invalid image", "metadata failure", "oversized request"} {
		t.Run(kind+" is rejected with safe diagnostics", func(t *testing.T) {
			store := &visualStore{}
			s := &server{authority: "127.0.0.1:7777", visual: visual.NewService(store, "user", t.TempDir())}
			req, _ := visualRequest(t, "png")
			want := 403
			switch kind {
			case "missing origin":
				req.Header.Del("Origin")
			case "foreign origin":
				req.Header.Set("Origin", "http://PRIVATE")
			case "duplicate origin":
				req.Header.Add("Origin", "http://127.0.0.1:7777")
			case "wrong host":
				req.Host = "PRIVATE"
			case "forwarded host":
				req.Host = "PRIVATE"
				req.Header.Set("X-Forwarded-Host", "127.0.0.1:7777")
			case "wrong method":
				req.Method = "PATCH"
				want = 405
			case "invalid image":
				req, _ = visualRequest(t, "svg")
				want = 400
			case "metadata failure":
				store.fail = true
				want = 500
			case "oversized request":
				var body bytes.Buffer
				writer := multipart.NewWriter(&body)
				part, _ := writer.CreateFormFile("image", "PRIVATE.png")
				_, _ = part.Write(make([]byte, maxVisualRequestBytes+1))
				_ = writer.Close()
				req = httptest.NewRequest("POST", "http://127.0.0.1:7777/visual/background", &body)
				req.Header.Set("Content-Type", writer.FormDataContentType())
				req.Header.Set("Origin", "http://127.0.0.1:7777")
				want = 413
			}
			w := httptest.NewRecorder()
			s.ServeHTTP(w, req)
			if w.Code != want {
				t.Fatalf("status = %d, want %d: %s", w.Code, want, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "PRIVATE") || strings.Contains(w.Body.String(), "/secret") {
				t.Fatalf("private diagnostic: %s", w.Body.String())
			}
		})
	}
	t.Run("rejects malformed darkness and preserves settings", func(t *testing.T) {
		store := &visualStore{settings: data.Visual{Darkness: 70}}
		s := &server{authority: "127.0.0.1:7777", visual: visual.NewService(store, "user", t.TempDir())}
		for _, body := range []string{`{}`, `{"darkness":96}`, `{"darkness":-1}`, `{"darkness":0.1}`, `{"darkness":20} {}`} {
			req := httptest.NewRequest("PUT", "http://127.0.0.1:7777/visual/settings", strings.NewReader(body))
			req.Header.Set("Origin", "http://127.0.0.1:7777")
			w := httptest.NewRecorder()
			s.ServeHTTP(w, req)
			if w.Code != 400 || store.settings.Darkness != 70 {
				t.Fatalf("body %s = %d, darkness %d", body, w.Code, store.settings.Darkness)
			}
		}
	})
}

type blockingVisualStore struct {
	entered   chan struct{}
	cancelled chan struct{}
	release   chan struct{}
}

func (s *blockingVisualStore) Load(ctx context.Context, _ string) (data.Visual, error) {
	close(s.entered)
	<-ctx.Done()
	close(s.cancelled)
	<-s.release
	return data.Visual{}, ctx.Err()
}
func (*blockingVisualStore) Save(context.Context, string, data.Visual) error { return nil }

func TestVisual_Shutdown(t *testing.T) {
	t.Run("cancels and joins requests before returning shared resources", func(t *testing.T) {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		store := &blockingVisualStore{entered: make(chan struct{}), cancelled: make(chan struct{}), release: make(chan struct{})}
		service := visual.NewService(store, "user", t.TempDir())
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			done <- Serve(ctx, listener, func(context.Context) tea.Model { return probeModel{} }, logging.Nop(), service)
		}()
		responseDone := make(chan struct{})
		go func() {
			defer close(responseDone)
			response, err := http.Get("http://" + listener.Addr().String() + "/visual")
			if err == nil {
				response.Body.Close()
			}
		}()
		select {
		case <-store.entered:
		case <-time.After(5 * time.Second):
			t.Fatal("request did not enter persistence")
		}
		cancel()
		select {
		case <-store.cancelled:
		case <-time.After(5 * time.Second):
			t.Fatal("request context was not cancelled")
		}
		select {
		case err := <-done:
			t.Fatalf("server returned before persistence joined: %v", err)
		default:
		}
		close(store.release)
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("server did not finish")
		}
		<-responseDone
	})
}

func TestVisual_MutationOrigins(t *testing.T) {
	for _, route := range []struct{ method, path string }{
		{"POST", "/visual/background"}, {"DELETE", "/visual/background"}, {"PUT", "/visual/settings"},
	} {
		for _, origin := range []string{"", "null", "http://localhost:7777", "http://127.0.0.1:7778"} {
			t.Run(route.method+" rejects "+origin, func(t *testing.T) {
				store := &visualStore{}
				s := &server{authority: "127.0.0.1:7777", visual: visual.NewService(store, "user", t.TempDir())}
				request := httptest.NewRequest(route.method, "http://127.0.0.1:7777"+route.path, nil)
				request.Header.Set("Origin", origin)
				response := httptest.NewRecorder()
				s.ServeHTTP(response, request)
				if response.Code != http.StatusForbidden {
					t.Fatalf("status = %d, want 403", response.Code)
				}
			})
		}
	}
}

func TestVisual_FramingSettings(t *testing.T) {
	t.Run("persists settings together and safely rejects invalid framing", func(t *testing.T) {
		store := &visualStore{settings: data.Visual{Darkness: 70, Framing: data.DefaultBackgroundFraming()}}
		server := &server{authority: "127.0.0.1:7777", visual: visual.NewService(store, "user", t.TempDir())}
		for _, body := range []string{
			`{"darkness":69,"framing":{"fit":"fill","zoom":150,"positionX":1200,"positionY":8500}}`,
			`{"darkness":30,"framing":{"fit":"PRIVATE","zoom":150,"positionX":1200,"positionY":8500}}`,
			`{"darkness":30,"framing":{"fit":"fit","zoom":301,"positionX":0,"positionY":0}}`,
			`{"darkness":30,"framing":{"fit":"fill","zoom":150,"positionX":-1,"positionY":0}}`,
		} {
			req := httptest.NewRequest("PUT", "http://127.0.0.1:7777/visual/settings", strings.NewReader(body))
			req.Header.Set("Origin", "http://127.0.0.1:7777")
			response := httptest.NewRecorder()
			server.ServeHTTP(response, req)
			want := 400
			if strings.Contains(body, `"darkness":69`) {
				want = 200
			}
			if response.Code != want {
				t.Fatalf("status = %d, want %d", response.Code, want)
			}
			if store.settings.Darkness != 69 || store.settings.Framing.Zoom != 150 || store.settings.Framing.PositionY != 8500 {
				t.Fatalf("saved settings = %+v", store.settings)
			}
			if strings.Contains(response.Body.String(), "PRIVATE") {
				t.Fatal("unsafe diagnostic")
			}
		}
	})
}
