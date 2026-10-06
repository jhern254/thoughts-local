package modelassets

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestInstaller_Client(t *testing.T) {
	t.Run("uses a dedicated bounded client without ambient credentials or proxies", func(t *testing.T) {
		installer, err := NewInstaller(openTestModelRoot(t))
		if err != nil {
			t.Fatal(err)
		}
		transport, ok := installer.httpClient.Transport.(*http.Transport)
		if !ok {
			t.Fatalf("transport got %T, want dedicated transport", installer.httpClient.Transport)
		}
		if installer.httpClient == http.DefaultClient || transport == http.DefaultTransport || installer.httpClient.Jar != nil || transport.Proxy != nil {
			t.Fatal("client inherited ambient HTTP state")
		}
		if !transport.DisableCompression || !transport.DisableKeepAlives {
			t.Fatal("compression or idle pooling is enabled")
		}
		for _, bound := range []struct {
			name             string
			duration         time.Duration
			expectedDuration time.Duration
		}{
			{name: "TLS handshake", duration: transport.TLSHandshakeTimeout, expectedDuration: 10 * time.Second},
			{name: "response headers", duration: transport.ResponseHeaderTimeout, expectedDuration: 30 * time.Second},
			{name: "download idle", duration: installer.downloadIdleTimeout, expectedDuration: 30 * time.Second},
			{name: "whole file", duration: installer.downloadTimeout, expectedDuration: 30 * time.Minute},
		} {
			if bound.duration != bound.expectedDuration {
				t.Errorf("%s timeout got %v, want %v", bound.name, bound.duration, bound.expectedDuration)
			}
		}
		assertError(t, installer.httpClient.CheckRedirect(nil, nil), http.ErrUseLastResponse)
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

type trackedResponseBody struct {
	io.Reader
	err    error
	closed bool
}

func (body *trackedResponseBody) Close() error {
	body.closed = true
	return body.err
}

func TestInstaller_Download(t *testing.T) {
	for _, testCase := range []struct {
		name              string
		statusCode        int
		declaredSizeBytes int64
		payload           string
		contentEncoding   string
		expectedError     error
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
		t.Run(testCase.name, func(t *testing.T) {
			body := &trackedResponseBody{Reader: strings.NewReader(testCase.payload)}
			client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode:    testCase.statusCode,
					Header:        http.Header{"Content-Encoding": []string{testCase.contentEncoding}},
					Body:          body,
					ContentLength: testCase.declaredSizeBytes,
				}, nil
			})}
			modelRoot := openTestModelRoot(t)
			manifest := fixtureModelManifest("https://example.test/file", "tiny")
			installer := newTestInstaller(t, modelRoot, []modelManifest{manifest}, client)
			_, err := installer.Install(context.Background(), manifest.ID)
			if testCase.expectedError == nil {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				assertError(t, err, testCase.expectedError)
				_, err = installer.VerifyInstallation(context.Background(), manifest.ID)
				assertError(t, err, ErrNotInstalled)
			}
			if !body.closed {
				t.Fatal("response body was not closed")
			}
			assertNoInstallerArtifacts(t, modelRoot)
		})
	}
	t.Run("rejects redirects without contacting destination", func(t *testing.T) {
		var targetRequests atomic.Int32
		target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetRequests.Add(1) }))
		defer target.Close()
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
		defer server.Close()
		modelRoot := openTestModelRoot(t)
		installer := newTestInstaller(t, modelRoot, []modelManifest{fixtureModelManifest(server.URL, "tiny")}, server.Client())
		_, err := installer.Install(context.Background(), "tiny")
		assertError(t, err, ErrDownload)
		if targetRequests.Load() != 0 {
			t.Fatal("redirect was followed")
		}
		assertNoInstallerArtifacts(t, modelRoot)
	})
	t.Run("network cause is preserved without printing sensitive text", func(t *testing.T) {
		cause := errors.New("sensitive URL or content")
		installer := newTestInstaller(t, openTestModelRoot(t), []modelManifest{fixtureModelManifest("https://example.test/file", "tiny")}, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, cause })})
		_, err := installer.Install(context.Background(), "tiny")
		assertError(t, err, ErrDownload)
		assertError(t, err, cause)
		if err.Error() != ErrDownload.Error() {
			t.Fatalf("unsafe error text: %v", err)
		}
	})
	t.Run("response closure must succeed", func(t *testing.T) {
		cause := errors.New("close failed")
		body := &trackedResponseBody{Reader: strings.NewReader("tiny"), err: cause}
		installer := newTestInstaller(t, openTestModelRoot(t), []modelManifest{fixtureModelManifest("https://example.test/file", "tiny")}, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    http.StatusOK,
				Body:          body,
				ContentLength: 4,
			}, nil
		})})
		_, err := installer.Install(context.Background(), "tiny")
		assertError(t, err, ErrDownload)
		assertError(t, err, cause)
	})
}

type contextBlockedBody struct {
	ctx  context.Context
	done chan struct{}
}

func (body *contextBlockedBody) Read([]byte) (int, error) {
	<-body.ctx.Done()
	return 0, body.ctx.Err()
}

func (body *contextBlockedBody) Close() error {
	close(body.done)
	return nil
}

func TestInstaller_Encoding(t *testing.T) {
	t.Run("rejects contradictory encoding headers", func(t *testing.T) {
		installer := newTestInstaller(t, openTestModelRoot(t), []modelManifest{fixtureModelManifest("https://example.test/file", "tiny")}, &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    200,
				Header:        http.Header{"Content-Encoding": []string{"identity", "gzip"}},
				Body:          io.NopCloser(strings.NewReader("tiny")),
				ContentLength: 4,
			}, nil
		})})
		_, err := installer.Install(context.Background(), "tiny")
		assertError(t, err, ErrDownload)
	})
}

func TestInstaller_DownloadCancellation(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		beginResponse func(http.ResponseWriter)
	}{
		{name: "headers", beginResponse: func(http.ResponseWriter) {}},
		{name: "body", beginResponse: func(response http.ResponseWriter) {
			response.Header().Set("Content-Length", "4")
			response.WriteHeader(http.StatusOK)
			response.(http.Flusher).Flush()
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			requestStarted := make(chan struct{})
			requestFinished := make(chan struct{})
			server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				defer close(requestFinished)
				testCase.beginResponse(response)
				close(requestStarted)
				<-request.Context().Done()
			}))
			defer server.Close()
			modelRoot := openTestModelRoot(t)
			installer := newTestInstaller(t, modelRoot, []modelManifest{fixtureModelManifest(server.URL, "tiny")}, server.Client())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			installResult := make(chan error, 1)
			go func() {
				_, err := installer.Install(ctx, "tiny")
				installResult <- err
			}()
			<-requestStarted
			cancel()
			assertError(t, <-installResult, context.Canceled)
			<-requestFinished
			assertNoInstallerArtifacts(t, modelRoot)
			_, err := installer.VerifyInstallation(context.Background(), "tiny")
			assertError(t, err, ErrNotInstalled)
		})
	}
	t.Run("idle timeout", func(t *testing.T) {
		requestFinished := make(chan struct{})
		server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			defer close(requestFinished)
			response.Header().Set("Content-Length", "4")
			response.WriteHeader(http.StatusOK)
			response.(http.Flusher).Flush()
			<-request.Context().Done()
		}))
		defer server.Close()
		modelRoot := openTestModelRoot(t)
		installer := newTestInstaller(t, modelRoot, []modelManifest{fixtureModelManifest(server.URL, "tiny")}, server.Client())
		installer.downloadIdleTimeout = 20 * time.Millisecond
		_, err := installer.Install(context.Background(), "tiny")
		assertError(t, err, context.Canceled)
		<-requestFinished
		assertNoInstallerArtifacts(t, modelRoot)
		_, err = installer.VerifyInstallation(context.Background(), "tiny")
		assertError(t, err, ErrNotInstalled)
	})
	t.Run("file timeout", func(t *testing.T) {
		responseClosed := make(chan struct{})
		client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode:    http.StatusOK,
				Body:          &contextBlockedBody{ctx: request.Context(), done: responseClosed},
				ContentLength: 4,
			}, nil
		})}
		modelRoot := openTestModelRoot(t)
		installer := newTestInstaller(t, modelRoot, []modelManifest{fixtureModelManifest("https://example.test/file", "tiny")}, client)
		installer.downloadTimeout = 100 * time.Millisecond
		_, err := installer.Install(context.Background(), "tiny")
		assertError(t, err, context.DeadlineExceeded)
		<-responseClosed
		assertNoInstallerArtifacts(t, modelRoot)
		_, err = installer.VerifyInstallation(context.Background(), "tiny")
		assertError(t, err, ErrNotInstalled)
	})
}
