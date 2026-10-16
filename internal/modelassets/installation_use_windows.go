package modelassets

import (
	"errors"
	"os"
	"runtime"
	"syscall"
	"unsafe"
)

func lockInstallationFile(lockFile *os.File, exclusive bool) (func() error, error) {
	process, err := syscall.GetCurrentProcess()
	if err != nil {
		return nil, err
	}
	// Own a non-inheritable raw handle: failed native cleanup must retain the
	// lock until process exit, even if the Go owner becomes unreachable.
	var lockHandle syscall.Handle
	err = syscall.DuplicateHandle(process, syscall.Handle(lockFile.Fd()), process, &lockHandle, 0, false, syscall.DUPLICATE_SAME_ACCESS)
	runtime.KeepAlive(lockFile)
	if err != nil {
		return nil, err
	}
	const lockFailImmediately = 1
	const lockExclusive = 2
	flags := uintptr(lockFailImmediately)
	if exclusive {
		flags |= lockExclusive
	}
	// Lock a byte of the permanent coordination file, not model contents.
	var offset syscall.Overlapped
	lockFileEx := syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")
	result, _, err := lockFileEx.Call(uintptr(lockHandle), flags, 0, 1, 0, uintptr(unsafe.Pointer(&offset)))
	if result != 0 {
		return func() error { return syscall.CloseHandle(lockHandle) }, nil
	}
	closeErr := syscall.CloseHandle(lockHandle)
	const errorLockViolation syscall.Errno = 33
	if err == errorLockViolation && closeErr == nil {
		return nil, ErrBusy
	}
	return nil, errors.Join(err, closeErr)
}
