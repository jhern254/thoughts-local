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

	"github.com/jhern254/go-thoughts/internal/appearance"
	"github.com/jhern254/go-thoughts/internal/data"
)

type appearanceStore struct {
	settings data.Appearance
	fail     bool
}

func (s *appearanceStore) Load(context.Context, string) (data.Appearance, error) {
	return s.settings, nil
}
func (s *appearanceStore) Save(_ context.Context, _ string, value data.Appearance) error {
	if s.fail {
		return errors.New("PRIVATE /secret/database.sqlite")
	}
	s.settings = value
	return nil
}
func appearanceRequest(t *testing.T, format string) (*http.Request, []byte) {
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
	_ = writer.Close()
	request := httptest.NewRequest("POST", "http://127.0.0.1:7777/appearance/background", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Origin", "http://127.0.0.1:7777")
	return request, imageBytes.Bytes()
}
func TestAppearance_Routes(t *testing.T) {
	for _, format := range []string{"jpeg", "png"} {
		t.Run("imports "+format+" by decoded content and serves only selected asset", func(t *testing.T) {
			store := &appearanceStore{}
			s := &server{authority: "127.0.0.1:7777", appearance: appearance.NewService(store, "user", t.TempDir())}
			req, want := appearanceRequest(t, format)
			w := httptest.NewRecorder()
			s.ServeHTTP(w, req)
			if w.Code != 200 {
				t.Fatalf("import = %d: %s", w.Code, w.Body.String())
			}
			w = httptest.NewRecorder()
			s.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:7777/appearance/background", nil))
			if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), want) {
				t.Fatalf("retrieval = %d, want original image", w.Code)
			}
			for _, path := range []string{"/appearance/../PRIVATE", "/appearance/background/../../PRIVATE", "/appearance/" + store.settings.BackgroundAsset} {
				w = httptest.NewRecorder()
				s.ServeHTTP(w, httptest.NewRequest("GET", "http://127.0.0.1:7777"+path, nil))
				if w.Code != 404 {
					t.Fatalf("%s = %d, want 404", path, w.Code)
				}
			}
			remove := httptest.NewRequest("DELETE", "http://127.0.0.1:7777/appearance/background", nil)
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
			store := &appearanceStore{}
			s := &server{authority: "127.0.0.1:7777", appearance: appearance.NewService(store, "user", t.TempDir())}
			req, _ := appearanceRequest(t, "png")
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
				req, _ = appearanceRequest(t, "svg")
				want = 400
			case "metadata failure":
				store.fail = true
				want = 500
			case "oversized request":
				var body bytes.Buffer
				writer := multipart.NewWriter(&body)
				part, _ := writer.CreateFormFile("image", "PRIVATE.png")
				_, _ = part.Write(make([]byte, maxAppearanceRequestBytes+1))
				_ = writer.Close()
				req = httptest.NewRequest("POST", "http://127.0.0.1:7777/appearance/background", &body)
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
		store := &appearanceStore{settings: data.Appearance{Darkness: 70}}
		s := &server{authority: "127.0.0.1:7777", appearance: appearance.NewService(store, "user", t.TempDir())}
		for _, body := range []string{`{}`, `{"darkness":96}`, `{"darkness":-1}`, `{"darkness":0.1}`, `{"darkness":20} {}`} {
			req := httptest.NewRequest("PUT", "http://127.0.0.1:7777/appearance/settings", strings.NewReader(body))
			req.Header.Set("Origin", "http://127.0.0.1:7777")
			w := httptest.NewRecorder()
			s.ServeHTTP(w, req)
			if w.Code != 400 || store.settings.Darkness != 70 {
				t.Fatalf("body %s = %d, darkness %d", body, w.Code, store.settings.Darkness)
			}
		}
	})
}

type blockingAppearanceStore struct {
	entered   chan struct{}
	cancelled chan struct{}
	release   chan struct{}
}

func (s *blockingAppearanceStore) Load(ctx context.Context, _ string) (data.Appearance, error) {
	close(s.entered)
	<-ctx.Done()
	close(s.cancelled)
	<-s.release
	return data.Appearance{}, ctx.Err()
}
func (*blockingAppearanceStore) Save(context.Context, string, data.Appearance) error { return nil }

func TestAppearance_Shutdown(t *testing.T) {
	t.Run("cancels and joins requests before returning shared resources", func(t *testing.T) {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		store := &blockingAppearanceStore{entered: make(chan struct{}), cancelled: make(chan struct{}), release: make(chan struct{})}
		service := appearance.NewService(store, "user", t.TempDir())
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)
		go func() {
			done <- Serve(ctx, listener, func(context.Context) tea.Model { return probeModel{} }, logging.Nop(), service)
		}()
		responseDone := make(chan struct{})
		go func() {
			defer close(responseDone)
			response, err := http.Get("http://" + listener.Addr().String() + "/appearance")
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

func TestAppearance_MutationOrigins(t *testing.T) {
	for _, route := range []struct{ method, path string }{
		{"POST", "/appearance/background"}, {"DELETE", "/appearance/background"}, {"PUT", "/appearance/settings"},
	} {
		for _, origin := range []string{"", "null", "http://localhost:7777", "http://127.0.0.1:7778"} {
			t.Run(route.method+" rejects "+origin, func(t *testing.T) {
				store := &appearanceStore{}
				s := &server{authority: "127.0.0.1:7777", appearance: appearance.NewService(store, "user", t.TempDir())}
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
