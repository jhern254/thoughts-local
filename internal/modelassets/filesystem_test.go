package modelassets

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestInstaller_Filesystem(t *testing.T) {
	t.Run("abandoned staging", func(t *testing.T) {
		modelRoot := openTestModelRoot(t)
		if err := modelRoot.Mkdir(".stage-abandoned", 0700); err != nil {
			t.Fatal(err)
		}
		installer := newTestInstaller(t, modelRoot, []modelManifest{fixtureModelManifest("https://example.test/file", "tiny")}, &http.Client{})
		_, err := installer.VerifyInstallation(context.Background(), "tiny")
		assertError(t, err, ErrNotInstalled)
	})
	for _, testCase := range []struct {
		name          string
		setup         func(*testing.T, *os.Root, string)
		expectedError error
	}{
		{name: "incomplete revision", setup: func(t *testing.T, modelRoot *os.Root, revisionDirectory string) {
			writeFixtureModelFile(t, modelRoot, revisionDirectory, "tiny")
		}, expectedError: ErrNotInstalled},
		{name: "corrupt file", setup: func(t *testing.T, modelRoot *os.Root, revisionDirectory string) {
			writeFixtureModelFile(t, modelRoot, revisionDirectory, "evil")
			if err := modelRoot.Mkdir(revisionDirectory+"/.complete", 0700); err != nil {
				t.Fatal(err)
			}
		}, expectedError: ErrIntegrity},
		{name: "symlink file", setup: func(t *testing.T, modelRoot *os.Root, revisionDirectory string) {
			if err := modelRoot.MkdirAll(revisionDirectory+"/.complete", 0700); err != nil {
				t.Fatal(err)
			}
			if err := modelRoot.Symlink("missing", revisionDirectory+"/model.bin"); err != nil {
				t.Skipf("symlink privilege unavailable: %v", err)
			}
		}, expectedError: ErrIntegrity},
		{name: "marker file", setup: func(t *testing.T, modelRoot *os.Root, revisionDirectory string) {
			writeFixtureModelFile(t, modelRoot, revisionDirectory, "tiny")
			if err := modelRoot.WriteFile(revisionDirectory+"/.complete", nil, 0600); err != nil {
				t.Fatal(err)
			}
		}, expectedError: ErrNotInstalled},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			modelRoot := openTestModelRoot(t)
			manifest := fixtureModelManifest("https://example.test/file", "tiny")
			testCase.setup(t, modelRoot, installationForManifest(manifest).Directory)
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Error("unexpected download")
				return nil, errors.New("unexpected")
			})}
			installer := newTestInstaller(t, modelRoot, []modelManifest{manifest}, client)
			_, err := installer.VerifyInstallation(context.Background(), manifest.ID)
			assertError(t, err, testCase.expectedError)
			_, err = installer.Install(context.Background(), manifest.ID)
			assertError(t, err, testCase.expectedError)
		})
	}
	t.Run("symlink escape cannot modify outside sentinel", func(t *testing.T) {
		outside := t.TempDir()
		sentinel := filepath.Join(outside, "sentinel")
		if err := os.WriteFile(sentinel, []byte("safe"), 0600); err != nil {
			t.Fatal(err)
		}
		modelRoot := openTestModelRoot(t)
		if err := modelRoot.Symlink(outside, "tiny"); err != nil {
			t.Skipf("symlink privilege unavailable: %v", err)
		}
		installer := newTestInstaller(t, modelRoot, []modelManifest{fixtureModelManifest("https://example.test/file", "tiny")}, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Error("unexpected download")
			return nil, errors.New("unexpected")
		})})
		_, err := installer.Install(context.Background(), "tiny")
		assertError(t, err, ErrFilesystem)
		b, err := os.ReadFile(sentinel)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != "safe" {
			t.Fatalf("outside bytes got %q, want safe", b)
		}
		names, err := os.ReadDir(outside)
		if err != nil {
			t.Fatal(err)
		}
		if len(names) != 1 {
			t.Fatalf("outside entries got %d, want 1", len(names))
		}
		assertNoInstallerArtifacts(t, modelRoot)
	})
	t.Run("failed newer revision preserves valid earlier revision", func(t *testing.T) {
		var corrupt atomic.Bool
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if corrupt.Load() {
				io.WriteString(w, "evil")
			} else {
				io.WriteString(w, "tiny")
			}
		}))
		defer server.Close()
		modelRoot := openTestModelRoot(t)
		previousManifest := fixtureModelManifest(server.URL, "tiny")
		installer := newTestInstaller(t, modelRoot, []modelManifest{previousManifest}, server.Client())
		if _, err := installer.Install(context.Background(), previousManifest.ID); err != nil {
			t.Fatal(err)
		}
		newerManifest := previousManifest
		newerManifest.Revision = "rev-2"
		newerInstaller := newTestInstaller(t, modelRoot, []modelManifest{newerManifest}, server.Client())
		corrupt.Store(true)
		_, err := newerInstaller.Install(context.Background(), newerManifest.ID)
		assertError(t, err, ErrIntegrity)
		if _, err := installer.VerifyInstallation(context.Background(), previousManifest.ID); err != nil {
			t.Fatal(err)
		}
		_, err = newerInstaller.VerifyInstallation(context.Background(), newerManifest.ID)
		assertError(t, err, ErrNotInstalled)
	})
	for _, testCase := range []struct {
		name          string
		injectFailure func(*Installer, error)
	}{
		{name: "file close", injectFailure: func(installer *Installer, cause error) {
			installer.closeFile = func(modelFile *os.File) error {
				modelFile.Close()
				return cause
			}
		}},
		{name: "rename", injectFailure: func(installer *Installer, cause error) {
			installer.publishStagingDirectory = func(string, string) error { return cause }
		}},
		{name: "marker", injectFailure: func(installer *Installer, cause error) {
			installer.createCompletionMarker = func(string) error { return cause }
		}},
	} {
		t.Run(testCase.name+" failure prevents discovery", func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) { io.WriteString(response, "tiny") }))
			defer server.Close()
			modelRoot := openTestModelRoot(t)
			installer := newTestInstaller(t, modelRoot, []modelManifest{fixtureModelManifest(server.URL, "tiny")}, server.Client())
			cause := errors.New("injected failure")
			testCase.injectFailure(installer, cause)
			_, err := installer.Install(context.Background(), "tiny")
			assertError(t, err, ErrFilesystem)
			assertError(t, err, cause)
			_, err = installer.VerifyInstallation(context.Background(), "tiny")
			assertError(t, err, ErrNotInstalled)
			assertNoInstallerArtifacts(t, modelRoot)
		})
	}
}

func TestInstaller_Recovery(t *testing.T) {
	t.Run("can install after a download failure releases the lock", func(t *testing.T) {
		var healthy atomic.Bool
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if healthy.Load() {
				io.WriteString(w, "tiny")
			} else {
				w.WriteHeader(503)
			}
		}))
		defer server.Close()
		modelRoot := openTestModelRoot(t)
		installer := newTestInstaller(t, modelRoot, []modelManifest{fixtureModelManifest(server.URL, "tiny")}, server.Client())
		_, err := installer.Install(context.Background(), "tiny")
		assertError(t, err, ErrDownload)
		healthy.Store(true)
		if _, err = installer.Install(context.Background(), "tiny"); err != nil {
			t.Fatal(err)
		}
		assertNoInstallerArtifacts(t, modelRoot)
	})
	t.Run("successful newer revision retains the earlier revision", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "tiny") }))
		defer server.Close()
		modelRoot := openTestModelRoot(t)
		previousManifest := fixtureModelManifest(server.URL, "tiny")
		installer := newTestInstaller(t, modelRoot, []modelManifest{previousManifest}, server.Client())
		if _, err := installer.Install(context.Background(), previousManifest.ID); err != nil {
			t.Fatal(err)
		}
		newerManifest := previousManifest
		newerManifest.Revision = "rev-2"
		newerInstaller := newTestInstaller(t, modelRoot, []modelManifest{newerManifest}, server.Client())
		if _, err := newerInstaller.Install(context.Background(), newerManifest.ID); err != nil {
			t.Fatal(err)
		}
		for _, installer := range []*Installer{installer, newerInstaller} {
			if _, err := installer.VerifyInstallation(context.Background(), previousManifest.ID); err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Run("stale lock remains busy without automatic takeover", func(t *testing.T) {
		modelRoot := openTestModelRoot(t)
		if err := modelRoot.Mkdir(installationLockDirectory, 0700); err != nil {
			t.Fatal(err)
		}
		installer := newTestInstaller(t, modelRoot, []modelManifest{fixtureModelManifest("https://example.test/file", "tiny")}, &http.Client{})
		_, err := installer.Install(context.Background(), "tiny")
		assertError(t, err, ErrBusy)
		if _, err := modelRoot.Stat(installationLockDirectory); err != nil {
			t.Fatalf("stale lock was removed: %v", err)
		}
	})
	t.Run("staging symlink escape cannot write or clean outside the root", func(t *testing.T) {
		outside := t.TempDir()
		sentinel := filepath.Join(outside, "model.bin")
		if err := os.WriteFile(sentinel, []byte("safe"), 0600); err != nil {
			t.Fatal(err)
		}
		modelRoot := openTestModelRoot(t)
		// Probe privileges before starting asynchronous HTTP work.
		if err := modelRoot.Symlink(outside, "probe"); err != nil {
			t.Skipf("symlink privilege unavailable: %v", err)
		}
		if err := modelRoot.Remove("probe"); err != nil {
			t.Fatal(err)
		}
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rootDirectory, err := modelRoot.Open(".")
			if err != nil {
				t.Error(err)
				return
			}
			names, err := rootDirectory.Readdirnames(-1)
			rootDirectory.Close()
			if err != nil {
				t.Error(err)
				return
			}
			for _, name := range names {
				if strings.HasPrefix(name, ".stage-") {
					if err := modelRoot.Remove(name + "/nested"); err != nil {
						t.Error(err)
						return
					}
					if err := modelRoot.Symlink(outside, name+"/nested"); err != nil {
						t.Error(err)
						return
					}
				}
			}
			io.WriteString(w, "tiny")
		}))
		defer server.Close()
		manifest := fixtureModelManifest(server.URL, "tiny")
		manifest.Files[0].RelativePath = "nested/model.bin"
		installer := newTestInstaller(t, modelRoot, []modelManifest{manifest}, server.Client())
		_, err := installer.Install(context.Background(), manifest.ID)
		assertError(t, err, ErrFilesystem)
		b, err := os.ReadFile(sentinel)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != "safe" {
			t.Fatalf("outside bytes got %q, want safe", b)
		}
		assertNoInstallerArtifacts(t, modelRoot)
	})
}

func writeFixtureModelFile(t *testing.T, modelRoot *os.Root, revisionDirectory, payload string) {
	t.Helper()
	if err := modelRoot.MkdirAll(revisionDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := modelRoot.WriteFile(revisionDirectory+"/model.bin", []byte(payload), 0600); err != nil {
		t.Fatal(err)
	}
}
