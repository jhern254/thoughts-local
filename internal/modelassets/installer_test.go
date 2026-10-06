package modelassets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
)

func openTestModelRoot(t *testing.T) *os.Root {
	t.Helper()
	modelRoot, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := modelRoot.Close(); err != nil {
			t.Error(err)
		}
	})
	return modelRoot
}

func TestInstaller_Unknown(t *testing.T) {
	t.Run("empty production catalog rejects IDs without filesystem changes", func(t *testing.T) {
		modelRoot := openTestModelRoot(t)
		installer, err := NewInstaller(modelRoot)
		if err != nil {
			t.Fatal(err)
		}
		for _, call := range []func(context.Context, ModelID) (Installation, error){installer.Install, installer.VerifyInstallation} {
			if _, err := call(context.Background(), "unknown"); !errors.Is(err, ErrUnknownModel) {
				t.Fatalf("got %v, want unknown model", err)
			}
		}
		files, err := modelRoot.Open(".")
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

func fixtureModelManifest(downloadURL, payload string) modelManifest {
	digest := sha256.Sum256([]byte(payload))
	return modelManifest{
		ID:       "tiny",
		Kind:     "speech",
		Runtime:  "fixture",
		Revision: "rev-1",
		Files: []modelFileManifest{{
			DownloadURL:       downloadURL,
			RelativePath:      "model.bin",
			ExpectedSizeBytes: int64(len(payload)),
			SHA256Hex:         hex.EncodeToString(digest[:]),
		}},
	}
}

func newTestInstaller(t *testing.T, modelRoot *os.Root, manifests []modelManifest, client *http.Client) *Installer {
	t.Helper()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	installer, err := newInstaller(modelRoot, manifests, client)
	if err != nil {
		t.Fatal(err)
	}
	return installer
}

func assertError(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}

func assertNoInstallerArtifacts(t *testing.T, modelRoot *os.Root) {
	t.Helper()
	rootDirectory, err := modelRoot.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	defer rootDirectory.Close()
	names, err := rootDirectory.Readdirnames(-1)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name == installationLockDirectory || strings.HasPrefix(name, ".stage-") {
			t.Fatalf("left temporary entry %q", name)
		}
	}
}

func readModelFile(t *testing.T, modelRoot *os.Root, relativePath string) string {
	t.Helper()
	modelFile, err := modelRoot.Open(relativePath)
	if err != nil {
		t.Fatal(err)
	}
	defer modelFile.Close()
	b, err := io.ReadAll(modelFile)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInstaller_Install(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		relativePaths []string
	}{
		{name: "one file", relativePaths: []string{"model.bin"}},
		{name: "multiple files", relativePaths: []string{"model.bin", "nested/config.bin"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				io.WriteString(w, "tiny")
			}))
			defer server.Close()
			manifest := fixtureModelManifest(server.URL, "tiny")
			fileTemplate := manifest.Files[0]
			manifest.Files = nil
			for _, relativePath := range testCase.relativePaths {
				modelFile := fileTemplate
				modelFile.RelativePath = relativePath
				manifest.Files = append(manifest.Files, modelFile)
			}
			modelRoot := openTestModelRoot(t)
			installer := newTestInstaller(t, modelRoot, []modelManifest{manifest}, server.Client())
			_, err := installer.VerifyInstallation(context.Background(), manifest.ID)
			assertError(t, err, ErrNotInstalled)
			result, err := installer.Install(context.Background(), manifest.ID)
			if err != nil {
				t.Fatal(err)
			}
			if result != installationForManifest(manifest) {
				t.Fatalf("got %+v, want %+v", result, installationForManifest(manifest))
			}
			for _, modelFile := range manifest.Files {
				if got := readModelFile(t, modelRoot, result.Directory+"/"+modelFile.RelativePath); got != "tiny" {
					t.Fatalf("bytes got %q, want tiny", got)
				}
			}
			got, err := installer.VerifyInstallation(context.Background(), manifest.ID)
			if err != nil || got != result {
				t.Fatalf("verification got %+v, %v, want %+v", got, err, result)
			}
			_, err = installer.Install(context.Background(), manifest.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got := requests.Load(); got != int32(len(manifest.Files)) {
				t.Fatalf("requests got %d, want %d", got, len(manifest.Files))
			}
			assertNoInstallerArtifacts(t, modelRoot)
		})
	}
	t.Run("unknown ID makes no requests", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Error("unexpected request")
			return nil, errors.New("unexpected")
		})}
		installer := newTestInstaller(t, openTestModelRoot(t), []modelManifest{fixtureModelManifest("https://example.test/file", "tiny")}, client)
		_, err := installer.Install(context.Background(), "unknown")
		assertError(t, err, ErrUnknownModel)
	})
	t.Run("discovery requires all files and the commit marker", func(t *testing.T) {
		secondStarted := make(chan struct{})
		release := make(chan struct{})
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/second" {
				close(secondStarted)
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
			}
			io.WriteString(w, "tiny")
		}))
		defer server.Close()
		manifest := fixtureModelManifest(server.URL, "tiny")
		modelFile := manifest.Files[0]
		modelFile.RelativePath = "second.bin"
		modelFile.DownloadURL = server.URL + "/second"
		manifest.Files = append(manifest.Files, modelFile)
		modelRoot := openTestModelRoot(t)
		installer := newTestInstaller(t, modelRoot, []modelManifest{manifest}, server.Client())
		beforeCommit := make(chan struct{})
		commit := make(chan struct{})
		installer.createCompletionMarker = func(markerPath string) error {
			close(beforeCommit)
			<-commit
			return modelRoot.Mkdir(markerPath, 0700)
		}
		result := make(chan error, 1)
		go func() {
			_, err := installer.Install(context.Background(), manifest.ID)
			result <- err
		}()
		<-secondStarted
		_, err := installer.VerifyInstallation(context.Background(), manifest.ID)
		assertError(t, err, ErrNotInstalled)
		close(release)
		<-beforeCommit
		_, err = installer.VerifyInstallation(context.Background(), manifest.ID)
		assertError(t, err, ErrNotInstalled)
		close(commit)
		if err := <-result; err != nil {
			t.Fatal(err)
		}
		if _, err = installer.VerifyInstallation(context.Background(), manifest.ID); err != nil {
			t.Fatal(err)
		}
	})
}

func TestInstaller_PublicationCancellation(t *testing.T) {
	t.Run("cancellation before marker prevents commit", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "tiny") }))
		defer server.Close()
		modelRoot := openTestModelRoot(t)
		installer := newTestInstaller(t, modelRoot, []modelManifest{fixtureModelManifest(server.URL, "tiny")}, server.Client())
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		installer.publishStagingDirectory = func(stagingDirectory, destinationPath string) error {
			err := modelRoot.Rename(stagingDirectory, destinationPath)
			cancel()
			return err
		}
		_, err := installer.Install(ctx, "tiny")
		assertError(t, err, context.Canceled)
		_, err = installer.VerifyInstallation(context.Background(), "tiny")
		assertError(t, err, ErrNotInstalled)
		assertNoInstallerArtifacts(t, modelRoot)
	})
	t.Run("completed commit wins concurrent cancellation", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "tiny") }))
		defer server.Close()
		modelRoot := openTestModelRoot(t)
		installer := newTestInstaller(t, modelRoot, []modelManifest{fixtureModelManifest(server.URL, "tiny")}, server.Client())
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		installer.createCompletionMarker = func(markerPath string) error {
			err := modelRoot.Mkdir(markerPath, 0700)
			cancel()
			return err
		}
		_, err := installer.Install(ctx, "tiny")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = installer.VerifyInstallation(context.Background(), "tiny"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("already canceled creates nothing", func(t *testing.T) {
		modelRoot := openTestModelRoot(t)
		installer := newTestInstaller(t, modelRoot, []modelManifest{fixtureModelManifest("https://example.test/file", "tiny")}, &http.Client{})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := installer.Install(ctx, "tiny")
		assertError(t, err, context.Canceled)
		assertNoInstallerArtifacts(t, modelRoot)
	})
}

func TestInstaller_Concurrency(t *testing.T) {
	t.Run("competing instances are busy and cannot interfere", func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})
		var requests atomic.Int32
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			close(started)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			io.WriteString(w, "tiny")
		}))
		defer server.Close()
		modelRoot := openTestModelRoot(t)
		manifest := fixtureModelManifest(server.URL, "tiny")
		installer := newTestInstaller(t, modelRoot, []modelManifest{manifest}, server.Client())
		other := newTestInstaller(t, modelRoot, []modelManifest{manifest}, server.Client())
		result := make(chan error, 1)
		go func() {
			_, err := installer.Install(context.Background(), manifest.ID)
			result <- err
		}()
		<-started
		_, err := other.Install(context.Background(), manifest.ID)
		assertError(t, err, ErrBusy)
		close(release)
		if err := <-result; err != nil {
			t.Fatal(err)
		}
		if _, err := other.Install(context.Background(), manifest.ID); err != nil {
			t.Fatal(err)
		}
		if requests.Load() != 1 {
			t.Fatalf("requests got %d, want 1", requests.Load())
		}
		assertNoInstallerArtifacts(t, modelRoot)
	})
}

func TestInstaller_FileDurability(t *testing.T) {
	t.Run("sync failure prevents publication and releases resources", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "tiny") }))
		defer server.Close()
		modelRoot := openTestModelRoot(t)
		installer := newTestInstaller(t, modelRoot, []modelManifest{fixtureModelManifest(server.URL, "tiny")}, server.Client())
		syncCause := errors.New("sync failed")
		syncError := &os.PathError{Op: "sync", Path: "sensitive local path", Err: syncCause}
		installer.syncFile = func(*os.File) error { return syncError }
		fileClosed := false
		installer.closeFile = func(modelFile *os.File) error {
			fileClosed = true
			return modelFile.Close()
		}
		installer.publishStagingDirectory = func(string, string) error {
			t.Error("published after sync failure")
			return nil
		}
		_, err := installer.Install(context.Background(), "tiny")
		assertError(t, err, ErrFilesystem)
		assertError(t, err, syncCause)
		var pathError *os.PathError
		if !errors.As(err, &pathError) || pathError != syncError {
			t.Fatalf("got %v, want retained sync cause", err)
		}
		if err.Error() != ErrFilesystem.Error() {
			t.Fatalf("unsafe error text: %v", err)
		}
		if !fileClosed {
			t.Error("downloaded file was not closed after sync failure")
		}
		_, err = installer.VerifyInstallation(context.Background(), "tiny")
		assertError(t, err, ErrNotInstalled)
		assertNoInstallerArtifacts(t, modelRoot)
		if _, err = modelRoot.Lstat("tiny/rev-1"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("target revision got %v, want absent", err)
		}
	})
	t.Run("sync failure for a newer revision preserves the valid earlier revision", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "tiny") }))
		defer server.Close()
		modelRoot := openTestModelRoot(t)
		previousManifest := fixtureModelManifest(server.URL, "tiny")
		previousInstaller := newTestInstaller(t, modelRoot, []modelManifest{previousManifest}, server.Client())
		previousInstallation, err := previousInstaller.Install(context.Background(), previousManifest.ID)
		if err != nil {
			t.Fatal(err)
		}
		newerManifest := previousManifest
		newerManifest.Revision = "rev-2"
		newerInstaller := newTestInstaller(t, modelRoot, []modelManifest{newerManifest}, server.Client())
		syncCause := errors.New("sync failed")
		newerInstaller.syncFile = func(*os.File) error { return syncCause }
		_, err = newerInstaller.Install(context.Background(), newerManifest.ID)
		assertError(t, err, ErrFilesystem)
		assertError(t, err, syncCause)
		verifiedInstallation, err := previousInstaller.VerifyInstallation(context.Background(), previousManifest.ID)
		if err != nil {
			t.Fatal(err)
		}
		if verifiedInstallation != previousInstallation {
			t.Fatalf("got %+v, want %+v", verifiedInstallation, previousInstallation)
		}
		_, err = newerInstaller.VerifyInstallation(context.Background(), newerManifest.ID)
		assertError(t, err, ErrNotInstalled)
		assertNoInstallerArtifacts(t, modelRoot)
	})
	t.Run("every verified file is synced and closed before staging is published", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "tiny") }))
		defer server.Close()
		modelRoot := openTestModelRoot(t)
		manifest := fixtureModelManifest(server.URL, "tiny")
		secondFile := manifest.Files[0]
		secondFile.RelativePath = "nested/config.bin"
		manifest.Files = append(manifest.Files, secondFile)
		installer := newTestInstaller(t, modelRoot, []modelManifest{manifest}, server.Client())
		syncedFiles := make(map[*os.File]bool)
		closedFiles := make(map[*os.File]bool)
		installer.syncFile = func(modelFile *os.File) error {
			if closedFiles[modelFile] {
				t.Error("file closed before sync")
			}
			info, err := modelFile.Stat()
			if err != nil {
				t.Fatal(err)
			}
			if info.Size() != 4 {
				t.Fatalf("size at sync got %d, want 4", info.Size())
			}
			syncedFiles[modelFile] = true
			return modelFile.Sync()
		}
		installer.closeFile = func(modelFile *os.File) error {
			if !syncedFiles[modelFile] {
				t.Error("file closed before sync")
			}
			closedFiles[modelFile] = true
			return modelFile.Close()
		}
		installer.publishStagingDirectory = func(stagingDirectory, destinationPath string) error {
			if len(syncedFiles) != 2 || len(closedFiles) != 2 {
				t.Errorf("synced %d and closed %d files, want 2 each", len(syncedFiles), len(closedFiles))
			}
			for modelFile := range syncedFiles {
				if _, err := modelFile.Stat(); !errors.Is(err, os.ErrClosed) {
					t.Errorf("file at publication got %v, want closed", err)
				}
			}
			return modelRoot.Rename(stagingDirectory, destinationPath)
		}
		if _, err := installer.Install(context.Background(), manifest.ID); err != nil {
			t.Fatal(err)
		}
		assertNoInstallerArtifacts(t, modelRoot)
	})
	t.Run("failed integrity verification never syncs a file", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "evil") }))
		defer server.Close()
		installer := newTestInstaller(t, openTestModelRoot(t), []modelManifest{fixtureModelManifest(server.URL, "tiny")}, server.Client())
		installer.syncFile = func(*os.File) error {
			t.Error("synced a file with invalid integrity")
			return nil
		}
		_, err := installer.Install(context.Background(), "tiny")
		assertError(t, err, ErrIntegrity)
	})
}
