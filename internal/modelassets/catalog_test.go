package modelassets

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCatalog_Validation(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		change func(*modelManifest)
	}{
		{"malformed digest", func(manifest *modelManifest) { manifest.Files[0].SHA256Hex = "bad" }},
		{"traversal", func(manifest *modelManifest) { manifest.Files[0].RelativePath = "../escape" }},
		{"absolute path", func(manifest *modelManifest) { manifest.Files[0].RelativePath = "/escape" }},
		{"duplicate files", func(manifest *modelManifest) { manifest.Files = append(manifest.Files, manifest.Files[0]) }},
		{"conflicting files", func(manifest *modelManifest) {
			modelFile := manifest.Files[0]
			modelFile.RelativePath += "/child"
			manifest.Files = append(manifest.Files, modelFile)
		}},
		{"case aliases", func(manifest *modelManifest) {
			modelFile := manifest.Files[0]
			modelFile.RelativePath = "MODEL.BIN"
			manifest.Files = append(manifest.Files, modelFile)
		}},
		{"reserved marker", func(manifest *modelManifest) { manifest.Files[0].RelativePath = ".complete/file" }},
		{"insecure URL", func(manifest *modelManifest) { manifest.Files[0].DownloadURL = "http://example.test/file" }},
		{"credentials", func(manifest *modelManifest) { manifest.Files[0].DownloadURL = "https://user:secret@example.test/file" }},
		{"fragment", func(manifest *modelManifest) { manifest.Files[0].DownloadURL = "https://example.test/file#fragment" }},
		{"missing host", func(manifest *modelManifest) { manifest.Files[0].DownloadURL = "https:///file" }},
		{"invalid identifier", func(manifest *modelManifest) { manifest.ID = "../bad" }},
		{"invalid revision", func(manifest *modelManifest) { manifest.Revision = "../bad" }},
		{"windows device", func(manifest *modelManifest) { manifest.Files[0].RelativePath = "CON.txt" }},
		{"backslash", func(manifest *modelManifest) { manifest.Files[0].RelativePath = `dir\file` }},
		{"zero size", func(manifest *modelManifest) { manifest.Files[0].ExpectedSizeBytes = 0 }},
		{"missing runtime", func(manifest *modelManifest) { manifest.Runtime = "" }},
		{"no files", func(manifest *modelManifest) { manifest.Files = nil }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			manifest := fixtureModelManifest("https://example.test/file", "tiny")
			testCase.change(&manifest)
			_, err := newInstaller(openTestModelRoot(t), []modelManifest{manifest}, &http.Client{})
			if !errors.Is(err, ErrInvalidCatalog) {
				t.Fatalf("got %v, want invalid catalog", err)
			}
		})
	}
}

func TestCatalog_Ownership(t *testing.T) {
	t.Run("rejects duplicate model IDs", func(t *testing.T) {
		manifest := fixtureModelManifest("https://example.test/file", "tiny")
		_, err := newInstaller(openTestModelRoot(t), []modelManifest{manifest, manifest}, &http.Client{})
		assertError(t, err, ErrInvalidCatalog)
	})
	t.Run("copies catalog files instead of sharing caller state", func(t *testing.T) {
		manifest := fixtureModelManifest("https://example.test/file", "tiny")
		installer := newTestInstaller(t, openTestModelRoot(t), []modelManifest{manifest}, &http.Client{})
		manifest.Files[0].RelativePath = "changed"
		if got := installer.catalog[manifest.ID].Files[0].RelativePath; got != "model.bin" {
			t.Fatalf("path got %q, want model.bin", got)
		}
	})
}

func TestCatalog_DigestCase(t *testing.T) {
	t.Run("accepts uppercase hex without changing digest semantics", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "tiny") }))
		defer server.Close()
		manifest := fixtureModelManifest(server.URL, "tiny")
		manifest.Files[0].SHA256Hex = strings.ToUpper(manifest.Files[0].SHA256Hex)
		installer := newTestInstaller(t, openTestModelRoot(t), []modelManifest{manifest}, server.Client())
		if _, err := installer.Install(context.Background(), manifest.ID); err != nil {
			t.Fatal(err)
		}
	})
}
