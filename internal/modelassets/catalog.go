// Package modelassets installs application-approved, pinned model files. The
// production catalog is intentionally empty pending separately reviewed manifests.
// Catalog maintenance must pin immutable upstream revisions, exact byte sizes and
// SHA-256 digests; callers cannot supply manifests, URLs or install destinations.
//
// The caller owns the root and must keep it open until all operations return.
// Only VerifyInstallation establishes installation validity; directory existence
// does not. After a process interruption, remove stale .install.lock, .stage-*
// directories and incomplete revisions manually, only while no installer is
// running. There is no automatic repair or lock takeover. Earlier revisions remain.
//
// Downloaded file contents are integrity-verified, synced and closed before
// publication. Directory metadata (including rename and marker creation) is not
// synced: portable directory synchronization is not available through this root
// API on all supported platforms. File Sync success does not guarantee that an
// installation or its completion marker survives a system crash or power loss.
package modelassets

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"net/url"
	"strings"
)

// ModelID selects a manifest from the application-approved catalog.
type ModelID string

// Installation describes a verified revision. Directory is relative to the root.
type Installation struct {
	ModelID   ModelID
	Revision  string
	Directory string
}

var (
	ErrUnknownModel   = errors.New("unknown model")
	ErrInvalidCatalog = errors.New("invalid model catalog")
	ErrBusy           = errors.New("model installation busy")
	ErrNotInstalled   = errors.New("model installation missing or incomplete")
	ErrDownload       = errors.New("model download failed")
	ErrIntegrity      = errors.New("model integrity verification failed")
	ErrFilesystem     = errors.New("model filesystem operation failed")
)

// categorizedError exposes only the fixed category through Error while
// retaining the underlying cause for errors.Is and errors.As. Unwrapped causes
// can contain sensitive URLs, paths or content and must not be logged.
type categorizedError struct {
	category error
	cause    error
}

func (categorized *categorizedError) Error() string { return categorized.category.Error() }
func (categorized *categorizedError) Unwrap() []error {
	return []error{categorized.category, categorized.cause}
}
func withErrorCategory(category, cause error) error {
	if cause == nil {
		return category
	}
	return &categorizedError{category: category, cause: cause}
}

type modelFileManifest struct {
	DownloadURL       string
	RelativePath      string
	ExpectedSizeBytes int64
	SHA256Hex         string
}
type modelManifest struct {
	ID          ModelID
	Kind        string
	Runtime     string
	Revision    string
	Files       []modelFileManifest
	Attribution string
}

func buildModelCatalog(manifests []modelManifest) (map[ModelID]modelManifest, error) {
	catalog := make(map[ModelID]modelManifest, len(manifests))
	for _, manifest := range manifests {
		if !isValidModelManifest(manifest) {
			return nil, ErrInvalidCatalog
		}
		if _, duplicateID := catalog[manifest.ID]; duplicateID {
			return nil, ErrInvalidCatalog
		}
		// Own the slice so later changes to the supplied manifest cannot change trust.
		manifest.Files = append([]modelFileManifest(nil), manifest.Files...)
		for fileIndex := range manifest.Files {
			manifest.Files[fileIndex].SHA256Hex = strings.ToLower(manifest.Files[fileIndex].SHA256Hex)
		}
		catalog[manifest.ID] = manifest
	}
	return catalog, nil
}

func isValidModelManifest(manifest modelManifest) bool {
	if !isPortableIdentifier(string(manifest.ID)) {
		return false
	}
	if !isPortableIdentifier(manifest.Revision) {
		return false
	}
	if !isPortableIdentifier(manifest.Kind) {
		return false
	}
	if !isPortableIdentifier(manifest.Runtime) {
		return false
	}
	if len(manifest.Files) == 0 {
		return false
	}
	for fileIndex, modelFile := range manifest.Files {
		if !isValidModelFileManifest(modelFile) {
			return false
		}
		for _, previousFile := range manifest.Files[:fileIndex] {
			if modelFilePathsConflict(modelFile.RelativePath, previousFile.RelativePath) {
				return false
			}
		}
	}
	return true
}

func isValidModelFileManifest(modelFile modelFileManifest) bool {
	downloadURL, err := url.Parse(modelFile.DownloadURL)
	if err != nil {
		return false
	}
	if downloadURL.Scheme != "https" {
		return false
	}
	if downloadURL.Hostname() == "" {
		return false
	}
	if downloadURL.User != nil {
		return false
	}
	if downloadURL.Fragment != "" {
		return false
	}
	if downloadURL.Opaque != "" {
		return false
	}
	if !isValidModelRelativePath(modelFile.RelativePath) {
		return false
	}
	if modelFile.ExpectedSizeBytes <= 0 {
		return false
	}
	// Overflow detection reads one extra byte; that bound must fit in an int64.
	if modelFile.ExpectedSizeBytes == math.MaxInt64 {
		return false
	}
	digest, err := hex.DecodeString(modelFile.SHA256Hex)
	if err != nil {
		return false
	}
	return len(digest) == sha256.Size
}

func modelFilePathsConflict(relativePath, previousRelativePath string) bool {
	normalizedPath := strings.ToLower(relativePath)
	normalizedPreviousPath := strings.ToLower(previousRelativePath)
	if normalizedPath == normalizedPreviousPath {
		return true
	}
	if strings.HasPrefix(normalizedPath, normalizedPreviousPath+"/") {
		return true
	}
	return strings.HasPrefix(normalizedPreviousPath, normalizedPath+"/")
}

// Portable identifiers use lowercase ASCII and avoid Windows device names and
// special path syntax, including case aliases across supported filesystems.
func isPortableIdentifier(identifier string) bool {
	if identifier == "" {
		return false
	}
	if !isLowercaseASCIIAlphanumeric(rune(identifier[0])) {
		return false
	}
	if strings.HasSuffix(identifier, ".") {
		return false
	}
	for _, character := range identifier {
		switch character {
		case '-', '_', '.':
			continue
		default:
			if !isLowercaseASCIIAlphanumeric(character) {
				return false
			}
		}
	}
	deviceName := strings.ToUpper(strings.SplitN(identifier, ".", 2)[0])
	switch deviceName {
	case "CON", "PRN", "AUX", "NUL":
		return false
	}
	if len(deviceName) == 4 {
		isNumberedDevice := strings.HasPrefix(deviceName, "COM") || strings.HasPrefix(deviceName, "LPT")
		if isNumberedDevice && deviceName[3] >= '0' && deviceName[3] <= '9' {
			return false
		}
	}
	return true
}
func isLowercaseASCIIAlphanumeric(character rune) bool {
	return character >= 'a' && character <= 'z' || character >= '0' && character <= '9'
}
func isValidModelRelativePath(relativePath string) bool {
	for _, component := range strings.Split(relativePath, "/") {
		if component == ".complete" {
			return false
		}
		if !isPortableIdentifier(component) {
			return false
		}
	}
	return true
}
