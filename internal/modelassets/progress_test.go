package modelassets

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestInstaller_InstallWithProgress(t *testing.T) {
	t.Run("reports actual cumulative bytes before publication and reuses verified files", func(t *testing.T) {
		modelRoot := openTestModelRoot(t)
		firstPayload := strings.Repeat("a", 70000)
		secondPayload := "second"
		manifest := fixtureModelManifest("https://example.test/first", firstPayload)
		secondFile := fixtureModelManifest("https://example.test/second", secondPayload).Files[0]
		secondFile.RelativePath = "second.bin"
		manifest.Files = append(manifest.Files, secondFile)
		requests := 0
		installer := newTestInstaller(t, modelRoot, []modelManifest{manifest}, &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			requests++
			payload := firstPayload
			if request.URL.Path == "/second" {
				payload = secondPayload
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(payload)), ContentLength: int64(len(payload))}, nil
		})})
		expectedTotal := int64(len(firstPayload) + len(secondPayload))
		previousBytes := int64(-1)
		updates := 0
		_, err := installer.InstallWithProgress(t.Context(), manifest.ID, func(progress DownloadProgress) {
			if progress.TotalBytes != expectedTotal || progress.DownloadedBytes < previousBytes || progress.DownloadedBytes > expectedTotal {
				t.Fatalf("unexpected progress: %+v", progress)
			}
			previousBytes = progress.DownloadedBytes
			updates++
			if _, err := installer.VerifyInstallation(t.Context(), manifest.ID); !errors.Is(err, ErrNotInstalled) {
				t.Fatalf("published before completed verification: %v", err)
			}
		})
		if err != nil {
			t.Fatal(err)
		}
		if updates < 4 || previousBytes != expectedTotal {
			t.Fatalf("updates=%d bytes=%d, want streaming progress ending at %d", updates, previousBytes, expectedTotal)
		}
		if _, err = installer.InstallWithProgress(t.Context(), manifest.ID, nil); err != nil {
			t.Fatal(err)
		}
		if requests != 2 {
			t.Fatalf("requests=%d, want 2", requests)
		}
	})
	t.Run("cancellation from progress stops installation and releases artifacts", func(t *testing.T) {
		modelRoot := openTestModelRoot(t)
		payload := strings.Repeat("a", 70000)
		installer := newTestInstaller(t, modelRoot, []modelManifest{fixtureModelManifest("https://example.test/file", payload)}, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(payload)), ContentLength: int64(len(payload))}, nil
		})})
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		_, err := installer.InstallWithProgress(ctx, "tiny", func(progress DownloadProgress) {
			if progress.DownloadedBytes > 0 {
				cancel()
			}
		})
		assertError(t, err, context.Canceled)
		assertNoInstallerArtifacts(t, modelRoot)
		_, err = installer.VerifyInstallation(t.Context(), "tiny")
		assertError(t, err, ErrNotInstalled)
	})
	t.Run("unknown IDs do not report progress", func(t *testing.T) {
		installer, err := NewInstaller(openTestModelRoot(t))
		if err != nil {
			t.Fatal(err)
		}
		_, err = installer.InstallWithProgress(t.Context(), "unknown", func(DownloadProgress) { t.Fatal("reported unknown model") })
		assertError(t, err, ErrUnknownModel)
	})
}
func TestDescribeModel(t *testing.T) {
	description, err := DescribeModel(MoonshineSmallStreamingEnglish)
	if err != nil {
		t.Fatal(err)
	}
	if description.ExpectedSizeBytes != 142300974 || description.DisplayName != "Moonshine Small Streaming English" {
		t.Fatalf("unexpected description: %+v", description)
	}
	if _, err := DescribeModel("unknown"); !errors.Is(err, ErrUnknownModel) {
		t.Fatalf("got %v, want unknown model", err)
	}
}

func TestCatalog_ProgressSize(t *testing.T) {
	t.Run("rejects combined model sizes that cannot fit the progress counter", func(t *testing.T) {
		manifest := fixtureModelManifest("https://example.test/file", "tiny")
		manifest.Files[0].ExpectedSizeBytes = 1 << 62
		secondFile := manifest.Files[0]
		secondFile.RelativePath = "second.bin"
		manifest.Files = append(manifest.Files, secondFile)
		if _, err := buildModelCatalog([]modelManifest{manifest}); !errors.Is(err, ErrInvalidCatalog) {
			t.Fatalf("got %v, want invalid catalog", err)
		}
	})
}
