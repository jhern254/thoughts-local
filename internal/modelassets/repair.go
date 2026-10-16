package modelassets

import (
	"context"
	"errors"
	"os"
)

// RepairWithProgress explicitly removes an invalid approved revision and installs
// its replacement. The mutation lock spans both operations: another installer
// cannot publish between deletion and replacement. A model held by a transcriber
// returns ErrBusy. Valid revisions are reused; missing revisions install normally.
// Cancellation after removal can leave the model absent. RemoveAll itself cannot
// be interrupted. Callers must release their model-use handles before repair.
func (installer *Installer) RepairWithProgress(ctx context.Context, modelID ModelID, reportProgress func(DownloadProgress)) (installation Installation, err error) {
	manifest, approved := installer.catalog[modelID]
	if !approved {
		return Installation{}, ErrUnknownModel
	}
	if err := ctx.Err(); err != nil {
		return Installation{}, err
	}
	if err := installer.acquireMutationLock(); err != nil {
		return Installation{}, err
	}
	defer installer.releaseMutationLock(&err)
	exclusiveUse, err := installer.acquireInstallationUse(ctx, manifest, true)
	if err != nil {
		return Installation{}, err
	}
	defer func() {
		if closeErr := exclusiveUse.Close(); closeErr != nil {
			err = withErrorCategory(ErrFilesystem, errors.Join(err, closeErr))
		}
	}()
	approvedInstallation := installationForManifest(manifest)
	// Root confinement alone permits aliases to other locations inside the root.
	// Refuse directory aliases so repair cannot remove another revision's files.
	for _, directory := range []string{string(manifest.ID), approvedInstallation.Directory} {
		metadata, statErr := installer.modelRoot.Lstat(directory)
		if errors.Is(statErr, os.ErrNotExist) {
			return installer.installManifestWithProgress(ctx, manifest, reportProgress)
		}
		if statErr != nil {
			return Installation{}, withErrorCategory(ErrFilesystem, statErr)
		}
		if !metadata.IsDir() {
			return Installation{}, ErrFilesystem
		}
	}
	installation, verifyErr := installer.verifyManifestInstallation(ctx, manifest)
	if verifyErr == nil {
		return installation, nil
	}
	if errors.Is(verifyErr, ErrFilesystem) {
		return Installation{}, verifyErr
	}
	if !errors.Is(verifyErr, ErrNotInstalled) && !errors.Is(verifyErr, ErrIntegrity) {
		return Installation{}, verifyErr
	}
	if err := ctx.Err(); err != nil {
		return Installation{}, err
	}
	// Revoke visibility before removing files, even if later removal fails.
	if err := installer.modelRoot.RemoveAll(approvedInstallation.Directory + "/.complete"); err != nil {
		return Installation{}, withErrorCategory(ErrFilesystem, err)
	}
	if err := installer.modelRoot.RemoveAll(approvedInstallation.Directory); err != nil {
		return Installation{}, withErrorCategory(ErrFilesystem, err)
	}
	return installer.installManifestWithProgress(ctx, manifest, reportProgress)
}
