package modelassets

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"time"
)

const installationLockDirectory = ".install.lock"

// Installer is safe for concurrent Install and VerifyInstallation calls after
// construction. The caller must keep the model root open until all calls return.
type Installer struct {
	modelRoot  *os.Root
	catalog    map[ModelID]modelManifest
	httpClient *http.Client
	// Narrow test seams; production operations stay confined to modelRoot.
	syncFile                func(*os.File) error
	closeFile               func(*os.File) error
	publishStagingDirectory func(string, string) error
	createCompletionMarker  func(string) error
	downloadIdleTimeout     time.Duration
	downloadTimeout         time.Duration
}

// NewInstaller uses a dedicated credential-free client and the pinned catalog.
func NewInstaller(modelRoot *os.Root) (*Installer, error) {
	return newInstaller(modelRoot, nil, newDownloadClient())
}
func newInstaller(modelRoot *os.Root, manifests []modelManifest, httpClient *http.Client) (*Installer, error) {
	if modelRoot == nil {
		return nil, ErrFilesystem
	}
	catalog, err := buildModelCatalog(manifests)
	if err != nil {
		return nil, err
	}
	return &Installer{
		modelRoot:               modelRoot,
		catalog:                 catalog,
		httpClient:              httpClient,
		syncFile:                (*os.File).Sync,
		closeFile:               (*os.File).Close,
		publishStagingDirectory: modelRoot.Rename,
		createCompletionMarker:  func(markerPath string) error { return modelRoot.Mkdir(markerPath, 0700) },
		downloadIdleTimeout:     30 * time.Second,
		downloadTimeout:         30 * time.Minute,
	}, nil
}

func installationForManifest(manifest modelManifest) Installation {
	return Installation{
		ModelID:   manifest.ID,
		Revision:  manifest.Revision,
		Directory: string(manifest.ID) + "/" + manifest.Revision,
	}
}

// VerifyInstallation requires a completion marker and verifies every required
// regular file's size and SHA-256. It reads and hashes the entire contents of all
// required files, so its cost scales with the model size. Directory existence
// alone never establishes validity, and verification never downloads files.
func (installer *Installer) VerifyInstallation(ctx context.Context, modelID ModelID) (Installation, error) {
	manifest, approved := installer.catalog[modelID]
	if !approved {
		return Installation{}, ErrUnknownModel
	}
	return installer.verifyManifestInstallation(ctx, manifest)
}
func (installer *Installer) verifyManifestInstallation(ctx context.Context, manifest modelManifest) (Installation, error) {
	if err := ctx.Err(); err != nil {
		return Installation{}, err
	}
	targetInstallation := installationForManifest(manifest)
	markerMetadata, err := installer.modelRoot.Lstat(targetInstallation.Directory + "/.complete")
	if errors.Is(err, os.ErrNotExist) {
		return Installation{}, ErrNotInstalled
	}
	if err != nil {
		return Installation{}, withErrorCategory(ErrFilesystem, err)
	}
	if !markerMetadata.IsDir() {
		return Installation{}, ErrNotInstalled
	}
	for _, modelFile := range manifest.Files {
		filePath := targetInstallation.Directory + "/" + modelFile.RelativePath
		if err := installer.verifyInstalledModelFile(ctx, filePath, modelFile); err != nil {
			return Installation{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return Installation{}, err
	}
	return targetInstallation, nil
}

func (installer *Installer) verifyInstalledModelFile(ctx context.Context, filePath string, fileManifest modelFileManifest) error {
	fileMetadata, err := installer.modelRoot.Lstat(filePath)
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotInstalled
	}
	if err != nil {
		return withErrorCategory(ErrFilesystem, err)
	}
	if !fileMetadata.Mode().IsRegular() {
		return ErrIntegrity
	}
	if fileMetadata.Size() != fileManifest.ExpectedSizeBytes {
		return ErrIntegrity
	}
	modelFile, err := installer.modelRoot.Open(filePath)
	if err != nil {
		return withErrorCategory(ErrFilesystem, err)
	}
	// Path metadata can change before Open; validate the descriptor actually read.
	fileMetadata, err = modelFile.Stat()
	if err == nil {
		if !fileMetadata.Mode().IsRegular() {
			err = ErrIntegrity
		}
		if fileMetadata.Size() != fileManifest.ExpectedSizeBytes {
			err = ErrIntegrity
		}
	}
	if err == nil {
		err = copyAndVerifyModelFile(ctx, modelFile, io.Discard, fileManifest, nil)
	}
	closeErr := installer.closeFile(modelFile)
	var lifecycleErr error
	if errors.Is(err, context.Canceled) {
		lifecycleErr = context.Canceled
	} else if errors.Is(err, context.DeadlineExceeded) {
		lifecycleErr = context.DeadlineExceeded
	}
	if lifecycleErr != nil {
		if closeErr != nil {
			err = errors.Join(err, withErrorCategory(ErrFilesystem, closeErr))
		}
		return withErrorCategory(lifecycleErr, err)
	}
	if err != nil {
		if errors.Is(err, ErrIntegrity) {
			return withErrorCategory(ErrIntegrity, errors.Join(err, closeErr))
		}
		return withErrorCategory(ErrFilesystem, errors.Join(err, closeErr))
	}
	if closeErr != nil {
		return withErrorCategory(ErrFilesystem, closeErr)
	}
	return nil
}

// Install serializes installs across processes using an exclusive directory lock.
// Files must be written, integrity-verified, synced and closed before staging is
// published. Only successful completion-marker creation makes a revision visible
// to VerifyInstallation; that commit wins concurrent cancellation. Cancellation
// observed before the commit prevents visibility and may leave an incomplete
// destination after rename. Directory metadata is not synced (see package docs).
// Existing revisions are fully verified and reused, never overwritten. Staging
// cleanup is best effort; lock release failures return a filesystem error.
func (installer *Installer) Install(ctx context.Context, modelID ModelID) (installedModel Installation, err error) {
	manifest, approved := installer.catalog[modelID]
	if !approved {
		return Installation{}, ErrUnknownModel
	}
	if err = ctx.Err(); err != nil {
		return Installation{}, err
	}
	if err = installer.modelRoot.Mkdir(installationLockDirectory, 0700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return Installation{}, ErrBusy
		}
		return Installation{}, withErrorCategory(ErrFilesystem, err)
	}
	defer func() {
		if releaseErr := installer.modelRoot.Remove(installationLockDirectory); releaseErr != nil {
			err = withErrorCategory(ErrFilesystem, errors.Join(err, releaseErr))
		}
	}()
	targetInstallation := installationForManifest(manifest)
	_, statErr := installer.modelRoot.Lstat(targetInstallation.Directory)
	if statErr == nil {
		return installer.verifyManifestInstallation(ctx, manifest)
	}
	if !errors.Is(statErr, os.ErrNotExist) {
		return Installation{}, withErrorCategory(ErrFilesystem, statErr)
	}

	stagingDirectory := ".stage-" + rand.Text()
	if err = installer.modelRoot.Mkdir(stagingDirectory, 0700); err != nil {
		return Installation{}, withErrorCategory(ErrFilesystem, err)
	}
	defer func() { _ = installer.modelRoot.RemoveAll(stagingDirectory) }()
	for _, modelFile := range manifest.Files {
		destinationPath := stagingDirectory + "/" + modelFile.RelativePath
		if err = installer.modelRoot.MkdirAll(path.Dir(destinationPath), 0700); err != nil {
			return Installation{}, withErrorCategory(ErrFilesystem, err)
		}
		if err = installer.downloadModelFile(ctx, destinationPath, modelFile); err != nil {
			return Installation{}, err
		}
	}
	if err = ctx.Err(); err != nil {
		return Installation{}, err
	}
	if err = installer.modelRoot.MkdirAll(string(manifest.ID), 0700); err != nil {
		return Installation{}, withErrorCategory(ErrFilesystem, err)
	}
	if err = installer.publishStagingDirectory(stagingDirectory, targetInstallation.Directory); err != nil {
		return Installation{}, withErrorCategory(ErrFilesystem, err)
	}
	if err = ctx.Err(); err != nil {
		return Installation{}, err
	}
	if err = installer.createCompletionMarker(targetInstallation.Directory + "/.complete"); err != nil {
		return Installation{}, withErrorCategory(ErrFilesystem, err)
	}
	return targetInstallation, nil
}
