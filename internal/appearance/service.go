// Package appearance owns one local browser background and its persisted settings.
package appearance

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

type Store interface {
	Load(context.Context, string) (data.Appearance, error)
	Save(context.Context, string, data.Appearance) error
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

func (s *Service) Load(ctx context.Context) (data.Appearance, error) {
	settings, err := s.store.Load(ctx, s.userID)
	if settings.Darkness < 0 || settings.Darkness > 95 {
		settings.Darkness = data.DefaultBackgroundDarkness
	}
	if settings.BackgroundAsset != "" && !validAsset(settings.BackgroundAsset) {
		return data.Appearance{}, errors.New("invalid background selection")
	}
	return settings, err
}

func validAsset(name string) bool {
	if len(name) != 36 || (filepath.Ext(name) != ".jpg" && filepath.Ext(name) != ".png") {
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

func (s *Service) Import(ctx context.Context, input io.Reader, darkness int) (data.Appearance, bool, error) {
	if !s.tryAcquire() {
		return data.Appearance{}, false, ErrBusy
	}
	defer s.release()
	if darkness < 0 || darkness > 95 {
		return data.Appearance{}, false, ErrDarkness
	}
	previous, err := s.Load(ctx)
	if err != nil {
		return data.Appearance{}, false, err
	}
	body, err := io.ReadAll(io.LimitReader(input, MaxImageBytes+1))
	if err != nil {
		return data.Appearance{}, false, err
	}
	format, err := validateImage(body)
	if err != nil {
		return data.Appearance{}, false, err
	}
	if err = ctx.Err(); err != nil {
		return data.Appearance{}, false, err
	}
	extension := ".png"
	if format == "jpeg" {
		extension = ".jpg"
	}
	id := make([]byte, 16)
	if _, err = rand.Read(id); err != nil {
		return data.Appearance{}, false, err
	}
	settings := data.Appearance{BackgroundAsset: hex.EncodeToString(id) + extension, Darkness: darkness}
	root, err := s.root()
	if err != nil {
		return data.Appearance{}, false, err
	}
	defer root.Close()
	file, err := root.OpenFile(settings.BackgroundAsset, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return data.Appearance{}, false, err
	}
	_, writeErr := file.Write(body)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err = errors.Join(writeErr, syncErr, closeErr, ctx.Err()); err == nil {
		err = s.store.Save(ctx, s.userID, settings)
	}
	if err != nil {
		_ = root.Remove(settings.BackgroundAsset)
		return data.Appearance{}, false, err
	}
	return settings, removeAsset(root, previous.BackgroundAsset), nil
}

func removeAsset(root *os.Root, name string) bool {
	if name == "" {
		return false
	}
	err := root.Remove(name)
	return err != nil && !errors.Is(err, os.ErrNotExist)
}

func (s *Service) SetDarkness(ctx context.Context, darkness int) (data.Appearance, error) {
	if !s.tryAcquire() {
		return data.Appearance{}, ErrBusy
	}
	defer s.release()
	if darkness < 0 || darkness > 95 {
		return data.Appearance{}, ErrDarkness
	}
	settings, err := s.Load(ctx)
	if err != nil {
		return settings, err
	}
	settings.Darkness = darkness
	return settings, s.store.Save(ctx, s.userID, settings)
}

func (s *Service) Remove(ctx context.Context) (data.Appearance, bool, error) {
	if !s.tryAcquire() {
		return data.Appearance{}, false, ErrBusy
	}
	defer s.release()
	settings, err := s.Load(ctx)
	if err != nil {
		return settings, false, err
	}
	previous := settings.BackgroundAsset
	settings.BackgroundAsset = ""
	if err = s.store.Save(ctx, s.userID, settings); err != nil {
		return data.Appearance{}, false, err
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
	// Keep selection and opening together so replacement cannot delete between them.
	select {
	case s.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}
	defer s.release()
	settings, err := s.Load(ctx)
	if err != nil {
		return nil, "", err
	}
	if settings.BackgroundAsset == "" {
		return nil, "", os.ErrNotExist
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
