package modelassets

import (
	"context"
	"errors"
	"testing"
)

func TestCatalog_MoonshineSmallStreamingEnglish(t *testing.T) {
	t.Run("selects the approved pinned model without downloading during verification", func(t *testing.T) {
		installer, err := NewInstaller(openTestModelRoot(t))
		if err != nil {
			t.Fatal(err)
		}
		manifest, approved := installer.catalog[MoonshineSmallStreamingEnglish]
		if !approved {
			t.Fatal("Small Streaming English is not approved")
		}
		if manifest.Revision != "quantized_26_08_21" || manifest.Runtime != "moonshine-voice-0.1.5" || manifest.Kind != "speech-to-text" {
			t.Fatalf("unexpected model identity: %+v", manifest)
		}
		if len(manifest.Files) != 8 {
			t.Fatalf("required files got %d, want 8", len(manifest.Files))
		}
		var totalSizeBytes int64
		for _, modelFile := range manifest.Files {
			totalSizeBytes += modelFile.ExpectedSizeBytes
			if modelFile.DownloadURL != moonshineSmallStreamingDownloadBase+modelFile.RelativePath {
				t.Fatalf("unapproved download URL for %s", modelFile.RelativePath)
			}
		}
		if totalSizeBytes != 142300974 {
			t.Fatalf("model size got %d, want 142300974", totalSizeBytes)
		}
		if _, err := installer.VerifyInstallation(context.Background(), MoonshineSmallStreamingEnglish); !errors.Is(err, ErrNotInstalled) {
			t.Fatalf("got %v, want missing installation", err)
		}
	})
}
