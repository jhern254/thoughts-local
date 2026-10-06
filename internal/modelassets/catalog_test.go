package modelassets

import (
	"context"
	"errors"
	"net/http"
	"os"
	"testing"
	"time"
)

func testRoot(t *testing.T) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Error(err)
		}
	})
	return root
}

func TestCatalog_Validation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*entry)
	}{
		{"malformed digest", func(e *entry) { e.Files[0].SHA256 = "bad" }},
		{"traversal", func(e *entry) { e.Files[0].Path = "../escape" }},
		{"absolute path", func(e *entry) { e.Files[0].Path = "/escape" }},
		{"duplicate files", func(e *entry) { e.Files = append(e.Files, e.Files[0]) }},
		{"conflicting files", func(e *entry) { f := e.Files[0]; f.Path += "/child"; e.Files = append(e.Files, f) }},
		{"case aliases", func(e *entry) { f := e.Files[0]; f.Path = "MODEL.BIN"; e.Files = append(e.Files, f) }},
		{"reserved marker", func(e *entry) { e.Files[0].Path = ".complete/file" }},
		{"insecure URL", func(e *entry) { e.Files[0].URL = "http://example.test/file" }},
		{"credentials", func(e *entry) { e.Files[0].URL = "https://user:secret@example.test/file" }},
		{"fragment", func(e *entry) { e.Files[0].URL = "https://example.test/file#fragment" }},
		{"missing host", func(e *entry) { e.Files[0].URL = "https:///file" }},
		{"invalid identifier", func(e *entry) { e.ID = "../bad" }},
		{"invalid revision", func(e *entry) { e.Revision = "../bad" }},
		{"windows device", func(e *entry) { e.Files[0].Path = "CON.txt" }},
		{"backslash", func(e *entry) { e.Files[0].Path = `dir\file` }},
		{"zero size", func(e *entry) { e.Files[0].Size = 0 }},
		{"missing runtime", func(e *entry) { e.Runtime = "" }},
		{"no files", func(e *entry) { e.Files = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := fixtureEntry("https://example.test/file", "tiny")
			tc.change(&e)
			_, err := newInstaller(testRoot(t), []entry{e}, &http.Client{})
			if !errors.Is(err, ErrInvalidCatalog) {
				t.Fatalf("got %v, want invalid catalog", err)
			}
		})
	}
}

func TestInstaller_Unknown(t *testing.T) {
	t.Run("empty production catalog rejects IDs without filesystem changes", func(t *testing.T) {
		root := testRoot(t)
		i, err := NewInstaller(root)
		if err != nil {
			t.Fatal(err)
		}
		for _, call := range []func(context.Context, ModelID) (Installation, error){i.Install, i.Lookup} {
			if _, err := call(context.Background(), "unknown"); !errors.Is(err, ErrUnknownModel) {
				t.Fatalf("got %v, want unknown model", err)
			}
		}
		files, err := root.Open(".")
		if err != nil {
			t.Fatal(err)
		}
		defer files.Close()
		names, err := files.Readdirnames(-1)
		if err != nil {
			t.Fatal(err)
		}
		if len(names) != 0 {
			t.Fatalf("created files: %v", names)
		}
	})
	t.Run("rejects nil root", func(t *testing.T) {
		if _, err := NewInstaller(nil); !errors.Is(err, ErrFilesystem) {
			t.Fatalf("got %v, want filesystem error", err)
		}
	})
}

func TestInstaller_Client(t *testing.T) {
	t.Run("uses a dedicated bounded client without ambient credentials or proxies", func(t *testing.T) {
		i, err := NewInstaller(testRoot(t))
		if err != nil {
			t.Fatal(err)
		}
		transport, ok := i.client.Transport.(*http.Transport)
		if !ok {
			t.Fatalf("transport got %T, want dedicated transport", i.client.Transport)
		}
		if i.client == http.DefaultClient || transport == http.DefaultTransport || i.client.Jar != nil || transport.Proxy != nil {
			t.Fatal("client inherited ambient HTTP state")
		}
		if !transport.DisableCompression || !transport.DisableKeepAlives {
			t.Fatal("compression or idle pooling is enabled")
		}
		if transport.TLSHandshakeTimeout != 10*time.Second || transport.ResponseHeaderTimeout != 30*time.Second || i.idleTimeout != 30*time.Second || i.fileTimeout != 30*time.Minute {
			t.Fatal("unexpected timeout bounds")
		}
		assertError(t, i.client.CheckRedirect(nil, nil), http.ErrUseLastResponse)
	})
	t.Run("rejects duplicate model IDs", func(t *testing.T) {
		e := fixtureEntry("https://example.test/file", "tiny")
		_, err := newInstaller(testRoot(t), []entry{e, e}, &http.Client{})
		assertError(t, err, ErrInvalidCatalog)
	})
	t.Run("copies catalog files instead of sharing caller state", func(t *testing.T) {
		e := fixtureEntry("https://example.test/file", "tiny")
		i := testInstaller(t, testRoot(t), []entry{e}, &http.Client{})
		e.Files[0].Path = "changed"
		if got := i.catalog[e.ID].Files[0].Path; got != "model.bin" {
			t.Fatalf("path got %q, want model.bin", got)
		}
	})
}
