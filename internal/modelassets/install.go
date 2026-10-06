package modelassets

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"time"
)

const lockName = ".install.lock"

func installation(e entry) Installation {
	return Installation{
		ModelID:   e.ID,
		Revision:  e.Revision,
		Directory: string(e.ID) + "/" + e.Revision,
	}
}

// Lookup verifies the completion marker and every required regular file. A
// missing marker is incomplete, even when all payload files are present.
func (i *Installer) Lookup(ctx context.Context, id ModelID) (Installation, error) {
	e, ok := i.catalog[id]
	if !ok {
		return Installation{}, ErrUnknownModel
	}
	return i.lookup(ctx, e)
}
func (i *Installer) lookup(ctx context.Context, e entry) (Installation, error) {
	if err := ctx.Err(); err != nil {
		return Installation{}, err
	}
	result := installation(e)
	marker, err := i.root.Lstat(result.Directory + "/.complete")
	if errors.Is(err, os.ErrNotExist) {
		return Installation{}, ErrNotInstalled
	}
	if err != nil {
		return Installation{}, failure(ErrFilesystem, err)
	}
	if !marker.IsDir() {
		return Installation{}, ErrNotInstalled
	}
	for _, a := range e.Files {
		name := result.Directory + "/" + a.Path
		info, err := i.root.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			return Installation{}, ErrNotInstalled
		}
		if err != nil {
			return Installation{}, failure(ErrFilesystem, err)
		}
		if !info.Mode().IsRegular() || info.Size() != a.Size {
			return Installation{}, ErrIntegrity
		}
		f, err := i.root.Open(name)
		if err != nil {
			return Installation{}, failure(ErrFilesystem, err)
		}
		// Check the opened descriptor too, rather than relying only on path metadata.
		info, err = f.Stat()
		if err == nil && (!info.Mode().IsRegular() || info.Size() != a.Size) {
			err = ErrIntegrity
		}
		if err == nil {
			err = verify(ctx, f, io.Discard, a, nil)
		}
		closeErr := i.closeFile(f)
		if err != nil {
			if errors.Is(err, ErrIntegrity) {
				return Installation{}, failure(ErrIntegrity, errors.Join(err, closeErr))
			}
			return Installation{}, failure(ErrFilesystem, errors.Join(err, closeErr))
		}
		if closeErr != nil {
			return Installation{}, failure(ErrFilesystem, closeErr)
		}
	}
	if err := ctx.Err(); err != nil {
		return Installation{}, err
	}
	return result, nil
}

// Install serializes publication across processes using an exclusive directory
// lock. A successful completion-marker creation commits the installation and
// wins concurrent cancellation. Existing incomplete or corrupt revisions are
// never overwritten. Cleanup is best effort for staging, but lock failures return
// a filesystem error. No operation may outlive the caller's root ownership.
func (i *Installer) Install(ctx context.Context, id ModelID) (result Installation, err error) {
	e, ok := i.catalog[id]
	if !ok {
		return Installation{}, ErrUnknownModel
	}
	if err = ctx.Err(); err != nil {
		return Installation{}, err
	}
	if err = i.root.Mkdir(lockName, 0700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return Installation{}, ErrBusy
		}
		return Installation{}, failure(ErrFilesystem, err)
	}
	defer func() {
		if releaseErr := i.root.Remove(lockName); releaseErr != nil {
			err = failure(ErrFilesystem, errors.Join(err, releaseErr))
		}
	}()
	final := installation(e)
	if _, statErr := i.root.Lstat(final.Directory); statErr == nil {
		return i.lookup(ctx, e)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return Installation{}, failure(ErrFilesystem, statErr)
	}
	stage := ".stage-" + rand.Text()
	if err = i.root.Mkdir(stage, 0700); err != nil {
		return Installation{}, failure(ErrFilesystem, err)
	}
	defer func() { _ = i.root.RemoveAll(stage) }()
	for _, a := range e.Files {
		name := stage + "/" + a.Path
		if err = i.root.MkdirAll(path.Dir(name), 0700); err != nil {
			return Installation{}, failure(ErrFilesystem, err)
		}
		if err = i.download(ctx, name, a); err != nil {
			return Installation{}, err
		}
	}
	if err = ctx.Err(); err != nil {
		return Installation{}, err
	}
	if err = i.root.MkdirAll(string(e.ID), 0700); err != nil {
		return Installation{}, failure(ErrFilesystem, err)
	}
	if err = i.rename(stage, final.Directory); err != nil {
		return Installation{}, failure(ErrFilesystem, err)
	}
	if err = ctx.Err(); err != nil {
		return Installation{}, err
	}
	if err = i.complete(final.Directory + "/.complete"); err != nil {
		return Installation{}, failure(ErrFilesystem, err)
	}
	return final, nil
}

func (i *Installer) download(ctx context.Context, name string, a asset) (err error) {
	fileCtx, cancel := context.WithTimeout(ctx, i.fileTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(fileCtx, http.MethodGet, a.URL, nil)
	if err != nil {
		return failure(ErrDownload, err)
	}
	resp, err := i.client.Do(req)
	if err != nil {
		return failure(ErrDownload, err)
	}
	// Every exit closes each resource exactly once and preserves closure failures.
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			err = failure(ErrDownload, errors.Join(err, closeErr))
		}
	}()
	encodings := resp.Header.Values("Content-Encoding")
	if resp.StatusCode != http.StatusOK || len(encodings) > 1 || (len(encodings) == 1 && encodings[0] != "" && encodings[0] != "identity") {
		return ErrDownload
	}
	if resp.ContentLength >= 0 && resp.ContentLength != a.Size {
		return ErrIntegrity
	}
	f, err := i.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return failure(ErrFilesystem, err)
	}
	defer func() {
		if closeErr := i.closeFile(f); closeErr != nil {
			err = failure(ErrFilesystem, errors.Join(err, closeErr))
		}
	}()
	// The request owns this watchdog. Cancel and join before files or root close.
	progress := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		timer := time.NewTimer(i.idleTimeout)
		defer timer.Stop()
		for {
			select {
			case <-fileCtx.Done():
				return
			case <-timer.C:
				cancel()
				return
			case <-progress:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(i.idleTimeout)
			}
		}
	}()
	defer func() {
		cancel()
		<-done
	}()
	err = verify(fileCtx, resp.Body, f, a, func() {
		select {
		case progress <- struct{}{}:
		default:
		}
	})
	if err != nil && !errors.Is(err, ErrIntegrity) && !errors.Is(err, ErrFilesystem) {
		return failure(ErrDownload, err)
	}
	return err
}

// verify bounds the stream independently of HTTP framing and uses fixed memory.
func verify(ctx context.Context, src io.Reader, dst io.Writer, a asset, progress func()) error {
	h := sha256.New()
	reader := io.LimitReader(src, a.Size+1)
	buf := make([]byte, 32*1024)
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := reader.Read(buf)
		if n > 0 {
			if progress != nil {
				progress()
			}
			size += int64(n)
			if size > a.Size {
				return ErrIntegrity
			}
			_, _ = h.Write(buf[:n])
			written, err := dst.Write(buf[:n])
			if err != nil {
				return failure(ErrFilesystem, err)
			}
			if written != n {
				return failure(ErrFilesystem, io.ErrShortWrite)
			}
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				if ctx.Err() != nil {
					return errors.Join(ctx.Err(), readErr)
				}
				if errors.Is(readErr, io.ErrUnexpectedEOF) {
					return failure(ErrIntegrity, readErr)
				}
				return readErr
			}
			break
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if size != a.Size || hex.EncodeToString(h.Sum(nil)) != a.SHA256 {
		return ErrIntegrity
	}
	return nil
}
