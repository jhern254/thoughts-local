package visual

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/jhern254/go-thoughts/internal/data"
)

type memoryStore struct {
	settings data.Visual
	fail     bool
}

func (s *memoryStore) Load(context.Context, string) (data.Visual, error) { return s.settings, nil }
func (s *memoryStore) Save(_ context.Context, _ string, value data.Visual) error {
	if s.fail {
		return errors.New("PRIVATE database path")
	}
	s.settings = value
	return nil
}
func samplePNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 12, 8))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestService_Background(t *testing.T) {
	for _, failure := range []string{"invalid image", "metadata failure", "cancelled request"} {
		t.Run(failure+" preserves previous selection", func(t *testing.T) {
			store := &memoryStore{}
			service := NewService(store, "user", t.TempDir())
			first, _, err := service.Import(t.Context(), bytes.NewReader(samplePNG(t)), 70)
			if err != nil {
				t.Fatal(err)
			}
			ctx := t.Context()
			body := samplePNG(t)
			switch failure {
			case "invalid image":
				body = []byte("PRIVATE invalid image")
			case "metadata failure":
				store.fail = true
			case "cancelled request":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if _, _, err = service.Import(ctx, bytes.NewReader(body), 30); err == nil {
				t.Fatal("replacement succeeded")
			}
			if store.settings != first {
				t.Fatalf("settings = %+v, want %+v", store.settings, first)
			}
			file, _, err := service.OpenBackground(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			got, err := io.ReadAll(file)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, samplePNG(t)) {
				t.Fatal("previous image changed")
			}
			entries, err := os.ReadDir(service.directory)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 {
				t.Fatalf("assets = %d, want 1", len(entries))
			}
		})
	}
	t.Run("replacement and removal clean selected assets", func(t *testing.T) {
		store := &memoryStore{}
		service := NewService(store, "user", filepath.Join(t.TempDir(), "appearance"))
		first, _, err := service.Import(t.Context(), bytes.NewReader(samplePNG(t)), 70)
		if err != nil {
			t.Fatal(err)
		}
		second, warning, err := service.Import(t.Context(), bytes.NewReader(samplePNG(t)), 35)
		if err != nil || warning {
			t.Fatalf("replace: %v, warning %v", err, warning)
		}
		if first.BackgroundAsset == second.BackgroundAsset {
			t.Fatal("reused asset")
		}
		if _, err = os.Stat(filepath.Join(service.directory, first.BackgroundAsset)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("old asset still exists: %v", err)
		}
		info, err := os.Stat(filepath.Join(service.directory, second.BackgroundAsset))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("permissions = %o, want 600", info.Mode().Perm())
		}
		settings, warning, err := service.Remove(t.Context())
		if err != nil || warning {
			t.Fatalf("remove: %v", err)
		}
		if settings.BackgroundAsset != "" || settings.Darkness != 35 {
			t.Fatalf("removed settings = %+v", settings)
		}
	})
	t.Run("refuses stored traversal and symlink", func(t *testing.T) {
		store := &memoryStore{settings: data.Visual{BackgroundAsset: "../../PRIVATE"}}
		dir := t.TempDir()
		service := NewService(store, "user", dir)
		if _, _, err := service.OpenBackground(t.Context()); err == nil {
			t.Fatal("accepted traversal")
		}
		store.settings.BackgroundAsset = "0123456789abcdef0123456789abcdef.png"
		if err := os.Symlink("/etc/passwd", filepath.Join(dir, store.settings.BackgroundAsset)); err != nil {
			t.Fatal(err)
		}
		if _, _, err := service.OpenBackground(t.Context()); err == nil {
			t.Fatal("accepted symlink")
		}
	})
}

func TestService_FilesystemFailure(t *testing.T) {
	t.Run("failed file creation preserves persisted selection", func(t *testing.T) {
		directory := filepath.Join(t.TempDir(), "appearance")
		if err := os.WriteFile(directory, []byte("not a directory"), 0600); err != nil {
			t.Fatal(err)
		}
		store := &memoryStore{settings: data.Visual{BackgroundAsset: "0123456789abcdef0123456789abcdef.png", Darkness: 70}}
		before := store.settings
		service := NewService(store, "user", directory)
		if _, _, err := service.Import(t.Context(), bytes.NewReader(samplePNG(t)), 35); err == nil {
			t.Fatal("import succeeded")
		}
		if store.settings != before {
			t.Fatalf("settings = %+v, want %+v", store.settings, before)
		}
	})
	t.Run("old file cleanup failure keeps successfully committed replacement", func(t *testing.T) {
		directory := t.TempDir()
		name := "0123456789abcdef0123456789abcdef.png"
		if err := os.Mkdir(filepath.Join(directory, name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, name, "block-removal"), nil, 0600); err != nil {
			t.Fatal(err)
		}
		store := &memoryStore{settings: data.Visual{BackgroundAsset: name, Darkness: 70}}
		service := NewService(store, "user", directory)
		got, warning, err := service.Import(t.Context(), bytes.NewReader(samplePNG(t)), 35)
		if err != nil || !warning {
			t.Fatalf("import = %v, cleanup warning %v; want success with warning", err, warning)
		}
		if store.settings != got || got.BackgroundAsset == name {
			t.Fatalf("committed settings = %+v", store.settings)
		}
	})
	t.Run("waiting image read respects request cancellation", func(t *testing.T) {
		service := NewService(&memoryStore{}, "user", t.TempDir())
		service.gate <- struct{}{}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, _, err := service.OpenBackground(ctx); !errors.Is(err, context.Canceled) {
			t.Fatalf("read = %v, want cancellation", err)
		}
	})
}
