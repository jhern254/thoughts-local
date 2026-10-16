package modelassets

import (
	"context"
	"errors"
	"os"
	"sync"
)

// InstallationUse keeps repair from deleting model files while native code uses
// them. Copies share ownership. Close only after all model mappings are released.
type InstallationUse struct {
	state *installationUseState
}

type installationUseState struct {
	mu       sync.Mutex
	release  func() error
	closeErr error
}

// AcquireInstallationUse takes shared access without waiting. It does not verify
// model contents. Hold it across verification, loading and native model lifetime.
// The small lock file is permanent: deleting it would split ownership between
// different filesystem objects. OS locks disappear when their process exits.
func (installer *Installer) AcquireInstallationUse(ctx context.Context, modelID ModelID) (*InstallationUse, error) {
	manifest, approved := installer.catalog[modelID]
	if !approved {
		return nil, ErrUnknownModel
	}
	return installer.acquireInstallationUse(ctx, manifest, false)
}

func (installer *Installer) acquireInstallationUse(ctx context.Context, manifest modelManifest, exclusive bool) (*InstallationUse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	lockPath := ".use-" + string(manifest.ID) + "-" + manifest.Revision + ".lock"
	if metadata, err := installer.modelRoot.Lstat(lockPath); err == nil {
		if !metadata.Mode().IsRegular() {
			return nil, ErrFilesystem
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, withErrorCategory(ErrFilesystem, err)
	}
	lockFile, err := installer.modelRoot.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, withErrorCategory(ErrFilesystem, err)
	}
	release, lockErr := lockInstallationFile(lockFile, exclusive)
	closeErr := lockFile.Close()
	if lockErr != nil || closeErr != nil {
		if release != nil {
			closeErr = errors.Join(closeErr, release())
		}
		if errors.Is(lockErr, ErrBusy) && closeErr == nil {
			return nil, ErrBusy
		}
		return nil, withErrorCategory(ErrFilesystem, errors.Join(lockErr, closeErr))
	}
	use := &InstallationUse{state: &installationUseState{release: release}}
	if err := ctx.Err(); err != nil {
		return nil, withErrorCategory(err, errors.Join(err, use.Close()))
	}
	return use, nil
}

// Close releases ownership once and returns the stored outcome on later calls.
func (use *InstallationUse) Close() error {
	if use == nil || use.state == nil {
		return nil
	}
	use.state.mu.Lock()
	defer use.state.mu.Unlock()
	if use.state.release != nil {
		if err := use.state.release(); err != nil {
			use.state.closeErr = withErrorCategory(ErrFilesystem, err)
		}
		use.state.release = nil
	}
	return use.state.closeErr
}
