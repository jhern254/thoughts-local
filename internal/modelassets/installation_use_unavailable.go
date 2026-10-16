//go:build !linux && (!darwin || ios) && !windows

package modelassets

import "os"

func lockInstallationFile(*os.File, bool) (func() error, error) {
	// Fail closed where native model lifetime coordination is not implemented.
	return nil, ErrFilesystem
}
