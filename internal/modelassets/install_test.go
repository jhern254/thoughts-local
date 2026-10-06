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
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fixtureEntry(url, payload string) entry {
	digest := sha256.Sum256([]byte(payload))
	return entry{
		ID:       "tiny",
		Kind:     "speech",
		Runtime:  "fixture",
		Revision: "rev-1",
		Files: []asset{{
			URL:    url,
			Path:   "model.bin",
			Size:   int64(len(payload)),
			SHA256: hex.EncodeToString(digest[:]),
		}},
	}
}
func testInstaller(t *testing.T, root *os.Root, entries []entry, client *http.Client) *Installer {
	t.Helper()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	i, err := newInstaller(root, entries, client)
	if err != nil {
		t.Fatal(err)
	}
	return i
}
func assertError(t *testing.T, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}
func assertNoLockOrStage(t *testing.T, root *os.Root) {
	t.Helper()
	f, err := root.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	names, err := f.Readdirnames(-1)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if name == lockName || strings.HasPrefix(name, ".stage-") {
			t.Fatalf("left temporary entry %q", name)
		}
	}
}
func readRoot(t *testing.T, root *os.Root, name string) string {
	t.Helper()
	f, err := root.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInstaller_Install(t *testing.T) {
	for _, multiple := range []bool{false, true} {
		name := "one file"
		if multiple {
			name = "multiple files"
		}
		t.Run(name, func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); io.WriteString(w, "tiny") }))
			defer server.Close()
			e := fixtureEntry(server.URL, "tiny")
			if multiple {
				f := e.Files[0]
				f.Path = "nested/config.bin"
				e.Files = append(e.Files, f)
			}
			root := testRoot(t)
			i := testInstaller(t, root, []entry{e}, server.Client())
			_, err := i.Lookup(context.Background(), e.ID)
			assertError(t, err, ErrNotInstalled)
			result, err := i.Install(context.Background(), e.ID)
			if err != nil {
				t.Fatal(err)
			}
			if result != installation(e) {
				t.Fatalf("got %+v, want %+v", result, installation(e))
			}
			for _, f := range e.Files {
				if got := readRoot(t, root, result.Directory+"/"+f.Path); got != "tiny" {
					t.Fatalf("bytes got %q, want tiny", got)
				}
			}
			got, err := i.Lookup(context.Background(), e.ID)
			if err != nil || got != result {
				t.Fatalf("lookup got %+v, %v, want %+v", got, err, result)
			}
			_, err = i.Install(context.Background(), e.ID)
			if err != nil {
				t.Fatal(err)
			}
			if got := requests.Load(); got != int32(len(e.Files)) {
				t.Fatalf("requests got %d, want %d", got, len(e.Files))
			}
			assertNoLockOrStage(t, root)
		})
	}
	t.Run("unknown ID makes no requests", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Error("unexpected request")
			return nil, errors.New("unexpected")
		})}
		i := testInstaller(t, testRoot(t), []entry{fixtureEntry("https://example.test/file", "tiny")}, client)
		_, err := i.Install(context.Background(), "unknown")
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
		e := fixtureEntry(server.URL, "tiny")
		f := e.Files[0]
		f.Path = "second.bin"
		f.URL = server.URL + "/second"
		e.Files = append(e.Files, f)
		root := testRoot(t)
		i := testInstaller(t, root, []entry{e}, server.Client())
		beforeCommit := make(chan struct{})
		commit := make(chan struct{})
		i.complete = func(name string) error { close(beforeCommit); <-commit; return root.Mkdir(name, 0700) }
		result := make(chan error, 1)
		go func() { _, err := i.Install(context.Background(), e.ID); result <- err }()
		<-secondStarted
		_, err := i.Lookup(context.Background(), e.ID)
		assertError(t, err, ErrNotInstalled)
		close(release)
		<-beforeCommit
		_, err = i.Lookup(context.Background(), e.ID)
		assertError(t, err, ErrNotInstalled)
		close(commit)
		if err := <-result; err != nil {
			t.Fatal(err)
		}
		if _, err = i.Lookup(context.Background(), e.ID); err != nil {
			t.Fatal(err)
		}
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type closeBody struct {
	io.Reader
	err    error
	closed bool
}

func (b *closeBody) Close() error { b.closed = true; return b.err }

func TestInstaller_Download(t *testing.T) {
	for _, tc := range []struct {
		name           string
		status         int
		length         int64
		body, encoding string
		want           error
	}{
		{"absent length succeeds", 200, -1, "tiny", "", nil},
		{"rejects status", 403, 4, "tiny", "", ErrDownload},
		{"rejects declared short length", 200, 3, "tiny", "", ErrIntegrity},
		{"rejects declared long length", 200, 5, "tiny", "", ErrIntegrity},
		{"rejects oversized body", 200, -1, "tiny!", "", ErrIntegrity},
		{"rejects truncated body", 200, -1, "tin", "", ErrIntegrity},
		{"rejects misleading exact length", 200, 4, "tiny!", "", ErrIntegrity},
		{"rejects wrong hash", 200, 4, "evil", "", ErrIntegrity},
		{"rejects encoding", 200, 4, "tiny", "gzip", ErrDownload},
		{"accepts identity encoding", 200, 4, "tiny", "identity", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := &closeBody{Reader: strings.NewReader(tc.body)}
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode:    tc.status,
					Header:        http.Header{"Content-Encoding": []string{tc.encoding}},
					Body:          body,
					ContentLength: tc.length,
				}, nil
			})}
			root := testRoot(t)
			e := fixtureEntry("https://example.test/file", "tiny")
			i := testInstaller(t, root, []entry{e}, client)
			_, err := i.Install(context.Background(), e.ID)
			if tc.want == nil {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				assertError(t, err, tc.want)
				_, err = i.Lookup(context.Background(), e.ID)
				assertError(t, err, ErrNotInstalled)
			}
			if !body.closed {
				t.Fatal("response body was not closed")
			}
			assertNoLockOrStage(t, root)
		})
	}
	t.Run("rejects redirects without contacting destination", func(t *testing.T) {
		var targetRequests atomic.Int32
		target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetRequests.Add(1) }))
		defer target.Close()
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
		defer server.Close()
		root := testRoot(t)
		i := testInstaller(t, root, []entry{fixtureEntry(server.URL, "tiny")}, server.Client())
		_, err := i.Install(context.Background(), "tiny")
		assertError(t, err, ErrDownload)
		if targetRequests.Load() != 0 {
			t.Fatal("redirect was followed")
		}
		assertNoLockOrStage(t, root)
	})
	t.Run("network cause is preserved without printing sensitive text", func(t *testing.T) {
		cause := errors.New("sensitive URL or content")
		i := testInstaller(t, testRoot(t), []entry{fixtureEntry("https://example.test/file", "tiny")}, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, cause })})
		_, err := i.Install(context.Background(), "tiny")
		assertError(t, err, ErrDownload)
		assertError(t, err, cause)
		if err.Error() != ErrDownload.Error() {
			t.Fatalf("unsafe error text: %v", err)
		}
	})
	t.Run("response closure must succeed", func(t *testing.T) {
		cause := errors.New("close failed")
		body := &closeBody{Reader: strings.NewReader("tiny"), err: cause}
		i := testInstaller(t, testRoot(t), []entry{fixtureEntry("https://example.test/file", "tiny")}, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, ContentLength: 4, Body: body}, nil
		})})
		_, err := i.Install(context.Background(), "tiny")
		assertError(t, err, ErrDownload)
		assertError(t, err, cause)
	})
}

func TestInstaller_Cancellation(t *testing.T) {
	for _, mode := range []string{"headers", "body", "idle timeout", "file timeout"} {
		t.Run(mode, func(t *testing.T) {
			started := make(chan struct{})
			finished := make(chan struct{})
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(finished)
				if mode != "headers" {
					w.Header().Set("Content-Length", "4")
					w.WriteHeader(200)
					w.(http.Flusher).Flush()
				}
				close(started)
				<-r.Context().Done()
			}))
			defer server.Close()
			root := testRoot(t)
			i := testInstaller(t, root, []entry{fixtureEntry(server.URL, "tiny")}, server.Client())
			if mode == "idle timeout" {
				i.idleTimeout = 20 * time.Millisecond
			}
			if mode == "file timeout" {
				i.fileTimeout = 100 * time.Millisecond
			}
			if mode == "file timeout" {
				server.Close()
				i.client = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					close(started)
					return &http.Response{StatusCode: 200, ContentLength: 4, Body: &deadlineBody{ctx: r.Context(), done: finished}}, nil
				})}
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() { _, err := i.Install(ctx, "tiny"); result <- err }()
			<-started
			if mode == "headers" || mode == "body" {
				cancel()
			}
			err := <-result
			if mode == "file timeout" {
				assertError(t, err, context.DeadlineExceeded)
			} else {
				assertError(t, err, context.Canceled)
			}
			<-finished
			assertNoLockOrStage(t, root)
			_, err = i.Lookup(context.Background(), "tiny")
			assertError(t, err, ErrNotInstalled)
		})
	}
	t.Run("cancellation before marker prevents commit", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "tiny") }))
		defer server.Close()
		root := testRoot(t)
		i := testInstaller(t, root, []entry{fixtureEntry(server.URL, "tiny")}, server.Client())
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		i.rename = func(from, to string) error { err := root.Rename(from, to); cancel(); return err }
		_, err := i.Install(ctx, "tiny")
		assertError(t, err, context.Canceled)
		_, err = i.Lookup(context.Background(), "tiny")
		assertError(t, err, ErrNotInstalled)
		assertNoLockOrStage(t, root)
	})
	t.Run("completed commit wins concurrent cancellation", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "tiny") }))
		defer server.Close()
		root := testRoot(t)
		i := testInstaller(t, root, []entry{fixtureEntry(server.URL, "tiny")}, server.Client())
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		i.complete = func(name string) error { err := root.Mkdir(name, 0700); cancel(); return err }
		_, err := i.Install(ctx, "tiny")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = i.Lookup(context.Background(), "tiny"); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("already canceled creates nothing", func(t *testing.T) {
		root := testRoot(t)
		i := testInstaller(t, root, []entry{fixtureEntry("https://example.test/file", "tiny")}, &http.Client{})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := i.Install(ctx, "tiny")
		assertError(t, err, context.Canceled)
		assertNoLockOrStage(t, root)
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
		root := testRoot(t)
		e := fixtureEntry(server.URL, "tiny")
		i := testInstaller(t, root, []entry{e}, server.Client())
		other := testInstaller(t, root, []entry{e}, server.Client())
		result := make(chan error, 1)
		go func() { _, err := i.Install(context.Background(), e.ID); result <- err }()
		<-started
		_, err := other.Install(context.Background(), e.ID)
		assertError(t, err, ErrBusy)
		close(release)
		if err := <-result; err != nil {
			t.Fatal(err)
		}
		if _, err := other.Install(context.Background(), e.ID); err != nil {
			t.Fatal(err)
		}
		if requests.Load() != 1 {
			t.Fatalf("requests got %d, want 1", requests.Load())
		}
		assertNoLockOrStage(t, root)
	})
}

func TestInstaller_Filesystem(t *testing.T) {
	for _, mode := range []string{"abandoned staging", "incomplete revision", "corrupt file", "symlink file", "marker file"} {
		t.Run(mode, func(t *testing.T) {
			root := testRoot(t)
			e := fixtureEntry("https://example.test/file", "tiny")
			dir := installation(e).Directory
			if mode == "abandoned staging" {
				if err := root.Mkdir(".stage-abandoned", 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := root.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := root.WriteFile(dir+"/model.bin", []byte("tiny"), 0600); err != nil {
					t.Fatal(err)
				}
				if mode != "incomplete revision" {
					if mode == "marker file" {
						if err := root.WriteFile(dir+"/.complete", nil, 0600); err != nil {
							t.Fatal(err)
						}
					} else {
						if err := root.Mkdir(dir+"/.complete", 0700); err != nil {
							t.Fatal(err)
						}
					}
				}
				if mode == "corrupt file" {
					if err := root.WriteFile(dir+"/model.bin", []byte("evil"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if mode == "symlink file" {
					if err := root.Remove(dir + "/model.bin"); err != nil {
						t.Fatal(err)
					}
					if err := root.Symlink("missing", dir+"/model.bin"); err != nil {
						t.Skipf("symlink privilege unavailable: %v", err)
					}
				}
			}
			i := testInstaller(t, root, []entry{e}, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Error("unexpected download")
				return nil, errors.New("unexpected")
			})})
			_, err := i.Lookup(context.Background(), e.ID)
			want := ErrNotInstalled
			if mode == "corrupt file" || mode == "symlink file" {
				want = ErrIntegrity
			}
			assertError(t, err, want)
			if mode != "abandoned staging" {
				_, err = i.Install(context.Background(), e.ID)
				assertError(t, err, want)
			}
		})
	}
	t.Run("symlink escape cannot modify outside sentinel", func(t *testing.T) {
		outside := t.TempDir()
		sentinel := filepath.Join(outside, "sentinel")
		if err := os.WriteFile(sentinel, []byte("safe"), 0600); err != nil {
			t.Fatal(err)
		}
		root := testRoot(t)
		if err := root.Symlink(outside, "tiny"); err != nil {
			t.Skipf("symlink privilege unavailable: %v", err)
		}
		i := testInstaller(t, root, []entry{fixtureEntry("https://example.test/file", "tiny")}, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			t.Error("unexpected download")
			return nil, errors.New("unexpected")
		})})
		_, err := i.Install(context.Background(), "tiny")
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
		assertNoLockOrStage(t, root)
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
		root := testRoot(t)
		old := fixtureEntry(server.URL, "tiny")
		i := testInstaller(t, root, []entry{old}, server.Client())
		if _, err := i.Install(context.Background(), old.ID); err != nil {
			t.Fatal(err)
		}
		next := old
		next.Revision = "rev-2"
		newer := testInstaller(t, root, []entry{next}, server.Client())
		corrupt.Store(true)
		_, err := newer.Install(context.Background(), next.ID)
		assertError(t, err, ErrIntegrity)
		if _, err := i.Lookup(context.Background(), old.ID); err != nil {
			t.Fatal(err)
		}
		_, err = newer.Lookup(context.Background(), next.ID)
		assertError(t, err, ErrNotInstalled)
	})
	for _, mode := range []string{"file close", "rename", "marker"} {
		t.Run(mode+" failure prevents discovery", func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "tiny") }))
			defer server.Close()
			root := testRoot(t)
			i := testInstaller(t, root, []entry{fixtureEntry(server.URL, "tiny")}, server.Client())
			cause := errors.New("injected failure")
			switch mode {
			case "file close":
				i.closeFile = func(f *os.File) error { f.Close(); return cause }
			case "rename":
				i.rename = func(string, string) error { return cause }
			case "marker":
				i.complete = func(string) error { return cause }
			}
			_, err := i.Install(context.Background(), "tiny")
			assertError(t, err, ErrFilesystem)
			assertError(t, err, cause)
			_, err = i.Lookup(context.Background(), "tiny")
			assertError(t, err, ErrNotInstalled)
			assertNoLockOrStage(t, root)
		})
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }
func TestVerify_Write(t *testing.T) {
	t.Run("short write is a filesystem failure", func(t *testing.T) {
		err := verify(context.Background(), strings.NewReader("tiny"), shortWriter{}, fixtureEntry("https://example.test/file", "tiny").Files[0], nil)
		assertError(t, err, ErrFilesystem)
		assertError(t, err, io.ErrShortWrite)
	})
}

func TestCatalog_DigestCase(t *testing.T) {
	t.Run("accepts uppercase hex without changing digest semantics", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "tiny") }))
		defer server.Close()
		e := fixtureEntry(server.URL, "tiny")
		e.Files[0].SHA256 = strings.ToUpper(e.Files[0].SHA256)
		i := testInstaller(t, testRoot(t), []entry{e}, server.Client())
		if _, err := i.Install(context.Background(), e.ID); err != nil {
			t.Fatal(err)
		}
	})
}

type truncatedReader struct{}

func (truncatedReader) Read(p []byte) (int, error) { return copy(p, "tin"), io.ErrUnexpectedEOF }
func TestVerify_Truncation(t *testing.T) {
	t.Run("framing truncation is an integrity failure", func(t *testing.T) {
		err := verify(context.Background(), truncatedReader{}, io.Discard, fixtureEntry("https://example.test/file", "tiny").Files[0], nil)
		assertError(t, err, ErrIntegrity)
		assertError(t, err, io.ErrUnexpectedEOF)
	})
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
		root := testRoot(t)
		i := testInstaller(t, root, []entry{fixtureEntry(server.URL, "tiny")}, server.Client())
		_, err := i.Install(context.Background(), "tiny")
		assertError(t, err, ErrDownload)
		healthy.Store(true)
		if _, err = i.Install(context.Background(), "tiny"); err != nil {
			t.Fatal(err)
		}
		assertNoLockOrStage(t, root)
	})
	t.Run("successful newer revision retains the earlier revision", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "tiny") }))
		defer server.Close()
		root := testRoot(t)
		old := fixtureEntry(server.URL, "tiny")
		i := testInstaller(t, root, []entry{old}, server.Client())
		if _, err := i.Install(context.Background(), old.ID); err != nil {
			t.Fatal(err)
		}
		next := old
		next.Revision = "rev-2"
		newer := testInstaller(t, root, []entry{next}, server.Client())
		if _, err := newer.Install(context.Background(), next.ID); err != nil {
			t.Fatal(err)
		}
		for _, installer := range []*Installer{i, newer} {
			if _, err := installer.Lookup(context.Background(), old.ID); err != nil {
				t.Fatal(err)
			}
		}
	})
	t.Run("stale lock remains busy without automatic takeover", func(t *testing.T) {
		root := testRoot(t)
		if err := root.Mkdir(lockName, 0700); err != nil {
			t.Fatal(err)
		}
		i := testInstaller(t, root, []entry{fixtureEntry("https://example.test/file", "tiny")}, &http.Client{})
		_, err := i.Install(context.Background(), "tiny")
		assertError(t, err, ErrBusy)
		if _, err := root.Stat(lockName); err != nil {
			t.Fatalf("stale lock was removed: %v", err)
		}
	})
	t.Run("staging symlink escape cannot write or clean outside the root", func(t *testing.T) {
		outside := t.TempDir()
		sentinel := filepath.Join(outside, "model.bin")
		if err := os.WriteFile(sentinel, []byte("safe"), 0600); err != nil {
			t.Fatal(err)
		}
		root := testRoot(t)
		// Probe privileges before starting asynchronous HTTP work.
		if err := root.Symlink(outside, "probe"); err != nil {
			t.Skipf("symlink privilege unavailable: %v", err)
		}
		if err := root.Remove("probe"); err != nil {
			t.Fatal(err)
		}
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			f, err := root.Open(".")
			if err != nil {
				t.Error(err)
				return
			}
			names, err := f.Readdirnames(-1)
			f.Close()
			if err != nil {
				t.Error(err)
				return
			}
			for _, name := range names {
				if strings.HasPrefix(name, ".stage-") {
					if err := root.Remove(name + "/nested"); err != nil {
						t.Error(err)
						return
					}
					if err := root.Symlink(outside, name+"/nested"); err != nil {
						t.Error(err)
						return
					}
				}
			}
			io.WriteString(w, "tiny")
		}))
		defer server.Close()
		e := fixtureEntry(server.URL, "tiny")
		e.Files[0].Path = "nested/model.bin"
		i := testInstaller(t, root, []entry{e}, server.Client())
		_, err := i.Install(context.Background(), e.ID)
		assertError(t, err, ErrFilesystem)
		b, err := os.ReadFile(sentinel)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != "safe" {
			t.Fatalf("outside bytes got %q, want safe", b)
		}
		assertNoLockOrStage(t, root)
	})
}

type countingReader struct {
	bytes  int
	reader io.Reader
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.bytes += n
	return n, err
}

type cancelReader struct{ cancel context.CancelFunc }

func (r cancelReader) Read(p []byte) (int, error) { r.cancel(); return copy(p, "tiny"), nil }
func TestVerify_Bounds(t *testing.T) {
	t.Run("reads at most the expected size plus one", func(t *testing.T) {
		r := &countingReader{reader: strings.NewReader(strings.Repeat("x", 100))}
		err := verify(context.Background(), r, io.Discard, fixtureEntry("https://example.test/file", "tiny").Files[0], nil)
		assertError(t, err, ErrIntegrity)
		if r.bytes != 5 {
			t.Fatalf("bytes read got %d, want 5", r.bytes)
		}
	})
	t.Run("checks cancellation while streaming discovery data", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		err := verify(ctx, cancelReader{cancel: cancel}, io.Discard, fixtureEntry("https://example.test/file", "tiny").Files[0], nil)
		assertError(t, err, context.Canceled)
	})
}

type deadlineBody struct {
	ctx  context.Context
	done chan struct{}
}

func (b *deadlineBody) Read([]byte) (int, error) { <-b.ctx.Done(); return 0, b.ctx.Err() }
func (b *deadlineBody) Close() error             { close(b.done); return nil }

func TestInstaller_Encoding(t *testing.T) {
	t.Run("rejects contradictory encoding headers", func(t *testing.T) {
		i := testInstaller(t, testRoot(t), []entry{fixtureEntry("https://example.test/file", "tiny")}, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    200,
				Header:        http.Header{"Content-Encoding": []string{"identity", "gzip"}},
				Body:          io.NopCloser(strings.NewReader("tiny")),
				ContentLength: 4,
			}, nil
		})})
		_, err := i.Install(context.Background(), "tiny")
		assertError(t, err, ErrDownload)
	})
}

func TestInstaller_FileDurability(t *testing.T) {
	t.Run("sync failure prevents publication and releases resources", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "tiny") }))
		defer server.Close()
		root := testRoot(t)
		installer := testInstaller(t, root, []entry{fixtureEntry(server.URL, "tiny")}, server.Client())
		syncCause := errors.New("sync failed")
		syncError := &os.PathError{Op: "sync", Path: "sensitive local path", Err: syncCause}
		installer.syncFile = func(*os.File) error { return syncError }
		fileClosed := false
		installer.closeFile = func(modelFile *os.File) error { fileClosed = true; return modelFile.Close() }
		installer.rename = func(string, string) error { t.Error("published after sync failure"); return nil }
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
		_, err = installer.Lookup(context.Background(), "tiny")
		assertError(t, err, ErrNotInstalled)
		assertNoLockOrStage(t, root)
		if _, err = root.Lstat("tiny/rev-1"); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("target revision got %v, want absent", err)
		}
	})
	t.Run("sync failure for a newer revision preserves the valid earlier revision", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "tiny") }))
		defer server.Close()
		root := testRoot(t)
		previousManifest := fixtureEntry(server.URL, "tiny")
		previousInstaller := testInstaller(t, root, []entry{previousManifest}, server.Client())
		previousInstallation, err := previousInstaller.Install(context.Background(), previousManifest.ID)
		if err != nil {
			t.Fatal(err)
		}
		newerManifest := previousManifest
		newerManifest.Revision = "rev-2"
		newerInstaller := testInstaller(t, root, []entry{newerManifest}, server.Client())
		syncCause := errors.New("sync failed")
		newerInstaller.syncFile = func(*os.File) error { return syncCause }
		_, err = newerInstaller.Install(context.Background(), newerManifest.ID)
		assertError(t, err, ErrFilesystem)
		assertError(t, err, syncCause)
		verifiedInstallation, err := previousInstaller.Lookup(context.Background(), previousManifest.ID)
		if err != nil {
			t.Fatal(err)
		}
		if verifiedInstallation != previousInstallation {
			t.Fatalf("got %+v, want %+v", verifiedInstallation, previousInstallation)
		}
		_, err = newerInstaller.Lookup(context.Background(), newerManifest.ID)
		assertError(t, err, ErrNotInstalled)
		assertNoLockOrStage(t, root)
	})
	t.Run("every verified file is synced and closed before staging is published", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "tiny") }))
		defer server.Close()
		root := testRoot(t)
		manifest := fixtureEntry(server.URL, "tiny")
		secondFile := manifest.Files[0]
		secondFile.Path = "nested/config.bin"
		manifest.Files = append(manifest.Files, secondFile)
		installer := testInstaller(t, root, []entry{manifest}, server.Client())
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
		installer.rename = func(stagingDirectory, destinationPath string) error {
			if len(syncedFiles) != 2 || len(closedFiles) != 2 {
				t.Errorf("synced %d and closed %d files, want 2 each", len(syncedFiles), len(closedFiles))
			}
			for modelFile := range syncedFiles {
				if _, err := modelFile.Stat(); !errors.Is(err, os.ErrClosed) {
					t.Errorf("file at publication got %v, want closed", err)
				}
			}
			return root.Rename(stagingDirectory, destinationPath)
		}
		if _, err := installer.Install(context.Background(), manifest.ID); err != nil {
			t.Fatal(err)
		}
		assertNoLockOrStage(t, root)
	})
	t.Run("failed integrity verification never syncs a file", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "evil") }))
		defer server.Close()
		installer := testInstaller(t, testRoot(t), []entry{fixtureEntry(server.URL, "tiny")}, server.Client())
		installer.syncFile = func(*os.File) error { t.Error("synced a file with invalid integrity"); return nil }
		_, err := installer.Install(context.Background(), "tiny")
		assertError(t, err, ErrIntegrity)
	})
}
