package modelassets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

func newDownloadClient() *http.Client {
	transport := &http.Transport{
		Proxy:                 nil,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		DisableCompression:    true,
		// No idle pooled connections outlive individual operations.
		DisableKeepAlives: true,
	}
	return &http.Client{
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func (installer *Installer) downloadModelFile(ctx context.Context, destinationPath string, fileManifest modelFileManifest) (err error) {
	downloadContext, cancelDownload := context.WithTimeout(ctx, installer.downloadTimeout)
	defer cancelDownload()
	request, err := http.NewRequestWithContext(downloadContext, http.MethodGet, fileManifest.DownloadURL, nil)
	if err != nil {
		return withErrorCategory(ErrDownload, err)
	}
	response, err := installer.httpClient.Do(request)
	if err != nil {
		return withErrorCategory(ErrDownload, err)
	}
	// Every exit closes each resource exactly once and preserves closure failures.
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			err = withErrorCategory(ErrDownload, errors.Join(err, closeErr))
		}
	}()
	if response.StatusCode != http.StatusOK {
		return ErrDownload
	}
	if !hasSupportedContentEncoding(response.Header) {
		return ErrDownload
	}
	if response.ContentLength >= 0 && response.ContentLength != fileManifest.ExpectedSizeBytes {
		return ErrIntegrity
	}
	modelFile, err := installer.modelRoot.OpenFile(destinationPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return withErrorCategory(ErrFilesystem, err)
	}
	defer func() {
		if closeErr := installer.closeFile(modelFile); closeErr != nil {
			err = withErrorCategory(ErrFilesystem, errors.Join(err, closeErr))
		}
	}()
	// The download request owns the watchdog. Cancel and join it before closing
	// files, closing the response body, or returning ownership to the caller.
	downloadProgress := make(chan struct{}, 1)
	watchdogStopped := make(chan struct{})
	go func() {
		defer close(watchdogStopped)
		idleTimer := time.NewTimer(installer.downloadIdleTimeout)
		defer idleTimer.Stop()
		for {
			select {
			case <-downloadContext.Done():
				return
			case <-idleTimer.C:
				cancelDownload()
				return
			case <-downloadProgress:
				if !idleTimer.Stop() {
					select {
					case <-idleTimer.C:
					default:
					}
				}
				idleTimer.Reset(installer.downloadIdleTimeout)
			}
		}
	}()
	defer func() {
		cancelDownload()
		<-watchdogStopped
	}()
	reportDownloadProgress := func() {
		select {
		case downloadProgress <- struct{}{}:
		default:
		}
	}
	err = copyAndVerifyModelFile(downloadContext, response.Body, modelFile, fileManifest, reportDownloadProgress)
	if err != nil {
		if errors.Is(err, ErrIntegrity) {
			return err
		}
		if errors.Is(err, ErrFilesystem) {
			return err
		}
		return withErrorCategory(ErrDownload, err)
	}
	if err := installer.syncFile(modelFile); err != nil {
		return withErrorCategory(ErrFilesystem, err)
	}
	return nil
}

func hasSupportedContentEncoding(headers http.Header) bool {
	encodings := headers.Values("Content-Encoding")
	switch len(encodings) {
	case 0:
		return true
	case 1:
		return encodings[0] == "" || encodings[0] == "identity"
	default:
		return false
	}
}

// copyAndVerifyModelFile copies and hashes a stream with fixed memory, enforcing
// the manifest's size and SHA-256 independently of HTTP framing. It reads at most
// expected size + one byte, writes only in-bound data, and checks cancellation
// between reads. Source errors are categorized by the download/discovery caller.
func copyAndVerifyModelFile(ctx context.Context, source io.Reader, destination io.Writer, fileManifest modelFileManifest, reportProgress func()) error {
	contentHash := sha256.New()
	boundedReader := io.LimitReader(source, fileManifest.ExpectedSizeBytes+1)
	buffer := make([]byte, 32*1024)
	var copiedSizeBytes int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		bytesRead, readErr := boundedReader.Read(buffer)
		if bytesRead > 0 {
			if reportProgress != nil {
				reportProgress()
			}
			copiedSizeBytes += int64(bytesRead)
			if copiedSizeBytes > fileManifest.ExpectedSizeBytes {
				return ErrIntegrity
			}
			_, _ = contentHash.Write(buffer[:bytesRead])
			bytesWritten, err := destination.Write(buffer[:bytesRead])
			if err != nil {
				return withErrorCategory(ErrFilesystem, err)
			}
			if bytesWritten != bytesRead {
				return withErrorCategory(ErrFilesystem, io.ErrShortWrite)
			}
		}
		if readErr == nil {
			continue
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if ctx.Err() != nil {
			return errors.Join(ctx.Err(), readErr)
		}
		if errors.Is(readErr, io.ErrUnexpectedEOF) {
			return withErrorCategory(ErrIntegrity, readErr)
		}
		return readErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if copiedSizeBytes != fileManifest.ExpectedSizeBytes {
		return ErrIntegrity
	}
	if hex.EncodeToString(contentHash.Sum(nil)) != fileManifest.SHA256Hex {
		return ErrIntegrity
	}
	return nil
}
