// Package visual owns one local browser background and its persisted settings.
package visual

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jhern254/go-thoughts/internal/data"
)

var ErrFraming = errors.New("invalid background framing")

type Store interface {
	Load(context.Context, string) (data.Visual, error)
	Save(context.Context, string, data.Visual) error
}

type Service struct {
	store     Store
	userID    string
	directory string
	gate      chan struct{}
}

func NewService(store Store, userID, directory string) *Service {
	return &Service{
		store:     store,
		userID:    userID,
		directory: directory,
		gate:      make(chan struct{}, 1),
	}
}

// Load waits for the owning mutation to settle. After a canceled import or
// lost response, browsers must reconcile committed selection, not read an
// intermediate value while processing or SQLite commit is still in progress.
func (s *Service) Load(ctx context.Context) (data.Visual, error) {
	if err := ctx.Err(); err != nil {
		return data.Visual{}, err
	}
	select {
	case s.gate <- struct{}{}:
	case <-ctx.Done():
		return data.Visual{}, ctx.Err()
	}
	defer s.release()
	return s.load(ctx)
}

func (s *Service) load(ctx context.Context) (data.Visual, error) {
	settings, err := s.store.Load(ctx, s.userID)
	if settings.VideoQuality == "" {
		settings.VideoQuality = data.DefaultVideoQuality
	}
	if err == nil && !VideoQuality(settings.VideoQuality).Valid() {
		return data.Visual{}, ErrVideoQuality
	}
	if settings.Darkness < 0 || settings.Darkness > 95 {
		settings.Darkness = data.DefaultBackgroundDarkness
	}
	if settings.SpeedPercent < 25 || settings.SpeedPercent > 100 {
		settings.SpeedPercent = 100
	}
	if !settings.Framing.Valid() {
		settings.Framing = data.DefaultBackgroundFraming()
	}
	if settings.BackgroundAsset != "" && !validAsset(settings.BackgroundAsset) {
		return data.Visual{}, errors.New("invalid background selection")
	}
	return settings, err
}

func validAsset(name string) bool {
	// Imports use 16 random bytes encoded as 32 lowercase hex characters, plus
	// a four-character .jpg/.png/.mp4 suffix. Accept only that application-owned name.
	if len(name) != 36 || (filepath.Ext(name) != ".jpg" && filepath.Ext(name) != ".png" && filepath.Ext(name) != ".mp4") {
		return false
	}
	_, err := hex.DecodeString(name[:32])
	return err == nil && name == strings.ToLower(name)
}

func (s *Service) root() (*os.Root, error) {
	if err := os.MkdirAll(s.directory, 0700); err != nil {
		return nil, err
	}
	return os.OpenRoot(s.directory)
}

func (s *Service) Import(ctx context.Context, input io.Reader, darkness int, framing data.BackgroundFraming, quality VideoQuality) (data.Visual, bool, error) {
	if !s.tryAcquire() {
		return data.Visual{}, false, ErrBusy
	}
	defer s.release()
	if darkness < 0 || darkness > 95 {
		return data.Visual{}, false, ErrDarkness
	}
	if !framing.Valid() {
		return data.Visual{}, false, ErrFraming
	}
	if quality != "" && !quality.Valid() {
		return data.Visual{}, false, ErrVideoQuality
	}
	previous, err := s.load(ctx)
	if err != nil {
		return data.Visual{}, false, err
	}
	body, err := io.ReadAll(io.LimitReader(input, MaxImageBytes+1))
	if err != nil {
		return data.Visual{}, false, err
	}
	// Sanitize before creating a replacement file; failure leaves the saved asset untouched.
	body, format, err := prepareImage(ctx, body)
	if err != nil {
		return data.Visual{}, false, err
	}
	if err = ctx.Err(); err != nil {
		return data.Visual{}, false, err
	}
	extension := ".png"
	if format == "jpeg" {
		extension = ".jpg"
	}
	settings := previous
	if quality != "" {
		settings.VideoQuality = string(quality)
	}
	settings.Darkness, settings.Framing = darkness, framing
	return s.commitBackground(ctx, previous, settings, extension, body, nil)
}

// Both video and poster must be durable before selection is committed. Files
// and SQLite are separate stores: crashes can leave unselected files, and an
// old-file deletion failure after commit is only a cleanup warning.
func (s *Service) commitBackground(ctx context.Context, previous, settings data.Visual, extension string, body, poster []byte) (data.Visual, bool, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return data.Visual{}, false, err
	}
	settings.BackgroundAsset = hex.EncodeToString(id) + extension
	root, err := s.root()
	if err != nil {
		return data.Visual{}, false, err
	}
	defer root.Close()
	write := func(name string, content []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, writeErr := file.Write(content)
		return errors.Join(writeErr, file.Sync(), file.Close(), ctx.Err())
	}
	err = write(settings.BackgroundAsset, body)
	if err == nil && poster != nil {
		err = write(posterAsset(settings.BackgroundAsset), poster)
	}
	if err == nil {
		err = s.store.Save(ctx, s.userID, settings)
	}
	if err != nil {
		removeAsset(root, settings.BackgroundAsset)
		return data.Visual{}, false, err
	}
	return settings, removeAsset(root, previous.BackgroundAsset), nil
}

// ApplyPreparedVideo commits the exact validated preview bytes without invoking
// media tools. The editor retains the temporary handle through the save, then
// closes it; failed saves leave both it and the selected background usable.
func (s *Service) ApplyPreparedVideo(ctx context.Context, prepared *PreparedVideo, darkness int, framing data.BackgroundFraming, speedPercent int, userPaused bool) (data.Visual, bool, error) {
	if !s.tryAcquire() {
		return data.Visual{}, false, ErrBusy
	}
	defer s.release()
	if darkness < 0 || darkness > 95 {
		return data.Visual{}, false, ErrDarkness
	}
	if !framing.Valid() {
		return data.Visual{}, false, ErrFraming
	}
	if speedPercent < 25 || speedPercent > 100 {
		return data.Visual{}, false, ErrPlayback
	}
	previous, err := s.load(ctx)
	if err != nil {
		return data.Visual{}, false, err
	}
	if prepared == nil || prepared.directory == "" {
		return data.Visual{}, false, ErrVideo
	}
	limits, err := prepared.info.VideoQuality.limits()
	if err != nil {
		return data.Visual{}, false, err
	}
	body, err := readBoundedVideoAsset(filepath.Join(prepared.directory, "display.mp4"), limits.MaxBytes)
	if err != nil {
		return data.Visual{}, false, err
	}
	poster, err := readBoundedVideoAsset(filepath.Join(prepared.directory, "poster.jpg"), MaxImageBytes)
	if err != nil {
		return data.Visual{}, false, err
	}
	settings := previous
	settings.Darkness, settings.Framing = darkness, framing
	settings.SpeedPercent, settings.UserPaused = speedPercent, userPaused
	settings.VideoQuality = string(prepared.info.VideoQuality)
	return s.commitBackground(ctx, previous, settings, ".mp4", body, poster)
}

func posterAsset(name string) string { return strings.TrimSuffix(name, ".mp4") + ".jpg" }

func removeAsset(root *os.Root, name string) bool {
	if name == "" {
		return false
	}
	err := root.Remove(name)
	warning := err != nil && !errors.Is(err, os.ErrNotExist)
	if filepath.Ext(name) == ".mp4" {
		err = root.Remove(posterAsset(name))
		warning = warning || (err != nil && !errors.Is(err, os.ErrNotExist))
	}
	return warning
}

func (s *Service) UpdateSettings(ctx context.Context, darkness int, framing data.BackgroundFraming, speedPercent int, userPaused bool, quality VideoQuality) (data.Visual, error) {
	if !s.tryAcquire() {
		return data.Visual{}, ErrBusy
	}
	defer s.release()
	if darkness < 0 || darkness > 95 {
		return data.Visual{}, ErrDarkness
	}
	settings, err := s.load(ctx)
	if err != nil {
		return settings, err
	}
	if !framing.Valid() {
		return data.Visual{}, ErrFraming
	}
	if speedPercent < 25 || speedPercent > 100 {
		return data.Visual{}, ErrPlayback
	}
	if quality != "" {
		if !quality.Valid() {
			return data.Visual{}, ErrVideoQuality
		}
		settings.VideoQuality = string(quality)
	}
	settings.SpeedPercent, settings.UserPaused = speedPercent, userPaused
	settings.Darkness = darkness
	settings.Framing = framing
	return settings, s.store.Save(ctx, s.userID, settings)
}

func (s *Service) Remove(ctx context.Context) (data.Visual, bool, error) {
	if !s.tryAcquire() {
		return data.Visual{}, false, ErrBusy
	}
	defer s.release()
	settings, err := s.load(ctx)
	if err != nil {
		return settings, false, err
	}
	previous := settings.BackgroundAsset
	settings.BackgroundAsset = ""
	if err = s.store.Save(ctx, s.userID, settings); err != nil {
		return data.Visual{}, false, err
	}
	if previous == "" {
		return settings, false, nil
	}
	root, err := s.root()
	if err != nil {
		return settings, true, nil
	}
	defer root.Close()
	return settings, removeAsset(root, previous), nil
}

func (s *Service) OpenBackground(ctx context.Context) (*os.File, string, error) {
	return s.openSelectedAsset(ctx, false)
}

func (s *Service) OpenPoster(ctx context.Context) (*os.File, string, error) {
	return s.openSelectedAsset(ctx, true)
}

func (s *Service) openSelectedAsset(ctx context.Context, poster bool) (*os.File, string, error) {
	// Keep selection and opening together so replacement cannot delete between them.
	select {
	case s.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}
	defer s.release()
	settings, err := s.load(ctx)
	if err != nil {
		return nil, "", err
	}
	if settings.BackgroundAsset == "" {
		return nil, "", os.ErrNotExist
	}
	if poster {
		if filepath.Ext(settings.BackgroundAsset) != ".mp4" {
			return nil, "", os.ErrNotExist
		}
		settings.BackgroundAsset = posterAsset(settings.BackgroundAsset)
	}
	root, err := os.OpenRoot(s.directory)
	if err != nil {
		return nil, "", err
	}
	defer root.Close()
	info, err := root.Lstat(settings.BackgroundAsset)
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() {
		return nil, "", os.ErrNotExist
	}
	file, err := root.Open(settings.BackgroundAsset)
	contentType := "image/png"
	if filepath.Ext(settings.BackgroundAsset) == ".jpg" {
		contentType = "image/jpeg"
	} else if filepath.Ext(settings.BackgroundAsset) == ".mp4" {
		contentType = "video/mp4"
	}
	return file, contentType, err
}

func (s *Service) tryAcquire() bool {
	select {
	case s.gate <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *Service) release() { <-s.gate }
