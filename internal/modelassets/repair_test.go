package modelassets

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path"
	"testing"
)

func TestInstaller_Repair(t *testing.T) {
	for _, damage := range []string{"missing marker", "wrong hash", "missing file", "wrong size", "nonregular file", "absent", "valid"} {
		t.Run(damage, func(t *testing.T) {
			requests := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; io.WriteString(w, "tiny") }))
			defer server.Close()
			root := openTestModelRoot(t)
			manifest := fixtureModelManifest(server.URL, "tiny")
			revision := installationForManifest(manifest).Directory
			installer := newTestInstaller(t, root, []modelManifest{manifest}, server.Client())
			if damage != "absent" {
				writeFixtureModelFile(t, root, revision, "tiny")
				if damage != "missing marker" {
					if err := root.Mkdir(revision+"/.complete", 0700); err != nil {
						t.Fatal(err)
					}
				}
				switch damage {
				case "wrong hash":
					if err := root.WriteFile(revision+"/model.bin", []byte("evil"), 0600); err != nil {
						t.Fatal(err)
					}
				case "wrong size":
					if err := root.WriteFile(revision+"/model.bin", []byte("short"), 0600); err != nil {
						t.Fatal(err)
					}
				case "missing file", "nonregular file":
					if err := root.Remove(revision + "/model.bin"); err != nil {
						t.Fatal(err)
					}
					if damage == "nonregular file" {
						if err := root.Mkdir(revision+"/model.bin", 0700); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			for _, sibling := range []string{"unrelated/user.txt", "tiny/older/model.bin", ".stage-abandoned/model.bin"} {
				if err := root.MkdirAll(path.Dir(sibling), 0700); err != nil {
					t.Fatal(err)
				}
				if err := root.WriteFile(sibling, []byte("safe"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			progressCalls := 0
			_, err := installer.RepairWithProgress(t.Context(), manifest.ID, func(DownloadProgress) {
				progressCalls++
				if use, err := installer.AcquireInstallationUse(t.Context(), manifest.ID); !errors.Is(err, ErrBusy) {
					if use != nil {
						use.Close()
					}
					t.Errorf("model loading during repair = %v, want busy", err)
				}
				if _, err := installer.Install(t.Context(), manifest.ID); !errors.Is(err, ErrBusy) {
					t.Errorf("install during repair = %v, want busy", err)
				}
				if _, err := installer.RepairWithProgress(t.Context(), manifest.ID, nil); !errors.Is(err, ErrBusy) {
					t.Errorf("repair during repair = %v, want busy", err)
				}
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := installer.VerifyInstallation(t.Context(), manifest.ID); err != nil {
				t.Fatal(err)
			}
			expectedRequests := 1
			if damage == "valid" {
				expectedRequests = 0
			}
			if requests != expectedRequests {
				t.Fatalf("requests=%d, want %d", requests, expectedRequests)
			}
			if damage != "valid" && progressCalls == 0 {
				t.Fatal("no progress")
			}
			for _, sibling := range []string{"unrelated/user.txt", "tiny/older/model.bin", ".stage-abandoned/model.bin"} {
				contents, err := root.ReadFile(sibling)
				if err != nil || string(contents) != "safe" {
					t.Fatalf("sibling changed: %v", err)
				}
			}
			if _, err := root.Stat(installationLockDirectory); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("lock remains: %v", err)
			}
		})
	}
	t.Run("unknown and canceled requests do not mutate", func(t *testing.T) {
		root := openTestModelRoot(t)
		installer := newTestInstaller(t, root, []modelManifest{fixtureModelManifest("https://example.test/file", "tiny")}, &http.Client{})
		_, err := installer.RepairWithProgress(t.Context(), "unknown", nil)
		assertError(t, err, ErrUnknownModel)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		_, err = installer.RepairWithProgress(ctx, "tiny", nil)
		assertError(t, err, context.Canceled)
		directory, err := root.Open(".")
		if err != nil {
			t.Fatal(err)
		}
		defer directory.Close()
		names, err := directory.Readdirnames(-1)
		if err != nil || len(names) != 0 {
			t.Fatalf("unexpected mutation: %v %v", names, err)
		}
	})
	t.Run("cancellation after removal leaves no installation and releases locks", func(t *testing.T) {
		root := openTestModelRoot(t)
		manifest := fixtureModelManifest("https://example.test/file", "tiny")
		installer := newTestInstaller(t, root, []modelManifest{manifest}, &http.Client{})
		writeFixtureModelFile(t, root, installationForManifest(manifest).Directory, "evil")
		ctx, cancel := context.WithCancel(t.Context())
		_, err := installer.RepairWithProgress(ctx, manifest.ID, func(DownloadProgress) { cancel() })
		assertError(t, err, context.Canceled)
		_, err = installer.VerifyInstallation(t.Context(), manifest.ID)
		assertError(t, err, ErrNotInstalled)
		assertNoInstallerArtifacts(t, root)
		use, err := installer.AcquireInstallationUse(t.Context(), manifest.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := use.Close(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestInstallationUse_Process(t *testing.T) {
	if directory := os.Getenv("THOUGHTS_MODEL_USE_TEST_ROOT"); directory != "" {
		root, err := os.OpenRoot(directory)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		installer := newTestInstaller(t, root, []modelManifest{fixtureModelManifest("https://example.test/file", "tiny")}, &http.Client{})
		use, err := installer.AcquireInstallationUse(t.Context(), "tiny")
		if err != nil {
			t.Fatal(err)
		}
		os.Stdout.Write([]byte{'R'})
		signal := make([]byte, 1)
		os.Stdin.Read(signal)
		if err := use.Close(); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Run("another process retains model ownership until exit", func(t *testing.T) {
		directory := t.TempDir()
		root, err := os.OpenRoot(directory)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Close()
		manifest := fixtureModelManifest("https://example.test/file", "tiny")
		installer := newTestInstaller(t, root, []modelManifest{manifest}, &http.Client{})
		writeFixtureModelFile(t, root, installationForManifest(manifest).Directory, "evil")
		command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestInstallationUse_Process$")
		command.Env = append(os.Environ(), "THOUGHTS_MODEL_USE_TEST_ROOT="+directory)
		stdout, err := command.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		stdin, err := command.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		defer stdin.Close()
		command.Stderr = os.Stderr
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		defer command.Process.Kill()
		signal := make([]byte, 1)
		if _, err := io.ReadFull(stdout, signal); err != nil || signal[0] != 'R' {
			t.Fatalf("child not ready: %v", err)
		}
		use, err := installer.AcquireInstallationUse(t.Context(), manifest.ID)
		if err != nil {
			t.Fatal(err)
		}
		if err := use.Close(); err != nil {
			t.Fatal(err)
		}
		if err := use.Close(); err != nil {
			t.Fatal(err)
		}
		_, err = installer.RepairWithProgress(t.Context(), manifest.ID, nil)
		assertError(t, err, ErrBusy)
		if err := command.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		_ = command.Wait()
		ctx, cancel := context.WithCancel(t.Context())
		_, err = installer.RepairWithProgress(ctx, manifest.ID, func(DownloadProgress) { cancel() })
		assertError(t, err, context.Canceled)
	})
}

func TestInstaller_RepairConfinement(t *testing.T) {
	for _, alias := range []string{"model directory", "revision directory", "nested file"} {
		t.Run(alias, func(t *testing.T) {
			root := openTestModelRoot(t)
			manifest := fixtureModelManifest("https://example.test/file", "tiny")
			revision := installationForManifest(manifest).Directory
			writeFixtureModelFile(t, root, "unrelated", "safe")
			linkPath := string(manifest.ID)
			linkTarget := "unrelated"
			if alias != "model directory" {
				if err := root.Mkdir(string(manifest.ID), 0700); err != nil {
					t.Fatal(err)
				}
				linkPath, linkTarget = revision, "../unrelated"
			}
			if alias == "nested file" {
				writeFixtureModelFile(t, root, revision, "evil")
				linkPath, linkTarget = revision+"/other", "../../unrelated"
			}
			if err := root.Symlink(linkTarget, linkPath); err != nil {
				t.Skipf("symlink privilege unavailable: %v", err)
			}
			installer := newTestInstaller(t, root, []modelManifest{manifest}, &http.Client{})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			_, err := installer.RepairWithProgress(ctx, manifest.ID, func(DownloadProgress) { cancel() })
			if alias == "nested file" {
				assertError(t, err, context.Canceled)
			} else {
				assertError(t, err, ErrFilesystem)
			}
			contents, err := root.ReadFile("unrelated/model.bin")
			if err != nil || string(contents) != "safe" {
				t.Fatalf("alias target changed: %v", err)
			}
		})
	}
	t.Run("outside symlink target is untouched", func(t *testing.T) {
		outside := t.TempDir()
		if err := os.WriteFile(outside+"/sentinel", []byte("safe"), 0600); err != nil {
			t.Fatal(err)
		}
		root := openTestModelRoot(t)
		if err := root.Symlink(outside, "tiny"); err != nil {
			t.Skipf("symlink privilege unavailable: %v", err)
		}
		installer := newTestInstaller(t, root, []modelManifest{fixtureModelManifest("https://example.test/file", "tiny")}, &http.Client{})
		_, err := installer.RepairWithProgress(t.Context(), "tiny", nil)
		assertError(t, err, ErrFilesystem)
		contents, err := os.ReadFile(outside + "/sentinel")
		if err != nil || string(contents) != "safe" {
			t.Fatalf("outside sentinel changed: %v", err)
		}
	})
	t.Run("filesystem failure is safe and releases mutation lock", func(t *testing.T) {
		root := openTestModelRoot(t)
		manifest := fixtureModelManifest("https://example.test/file", "tiny")
		writeFixtureModelFile(t, root, installationForManifest(manifest).Directory, "tiny")
		if err := root.Mkdir(installationForManifest(manifest).Directory+"/.complete", 0700); err != nil {
			t.Fatal(err)
		}
		installer := newTestInstaller(t, root, []modelManifest{manifest}, &http.Client{})
		cause := errors.New("private path")
		installer.closeFile = func(file *os.File) error { file.Close(); return cause }
		_, err := installer.RepairWithProgress(t.Context(), manifest.ID, nil)
		assertError(t, err, ErrFilesystem)
		assertError(t, err, cause)
		if err.Error() != ErrFilesystem.Error() {
			t.Fatalf("unsafe public error: %v", err)
		}
		assertNoInstallerArtifacts(t, root)
		if _, err := root.Stat(installationForManifest(manifest).Directory + "/model.bin"); err != nil {
			t.Fatal("removed model after filesystem failure")
		}
	})
}
