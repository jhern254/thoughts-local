//go:build linux || (darwin && !ios)

package modelassets

import (
	"errors"
	"os"
	"runtime"
	"syscall"
)

func lockInstallationFile(lockFile *os.File, exclusive bool) (func() error, error) {
	// A raw duplicate has explicit lifetime, without os.File's GC cleanup. If
	// native cleanup fails, abandoning this handle keeps repair blocked until exit.
	syscall.ForkLock.RLock()
	descriptor, err := syscall.Dup(int(lockFile.Fd()))
	if err == nil {
		syscall.CloseOnExec(descriptor)
	}
	syscall.ForkLock.RUnlock()
	runtime.KeepAlive(lockFile)
	if err != nil {
		return nil, err
	}
	mode := syscall.LOCK_SH | syscall.LOCK_NB
	if exclusive {
		mode = syscall.LOCK_EX | syscall.LOCK_NB
	}
	if err := syscall.Flock(descriptor, mode); err != nil {
		closeErr := syscall.Close(descriptor)
		if errors.Is(err, syscall.EWOULDBLOCK) && closeErr == nil {
			return nil, ErrBusy
		}
		return nil, errors.Join(err, closeErr)
	}
	return func() error { return syscall.Close(descriptor) }, nil
}
