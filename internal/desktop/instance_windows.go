package desktop

import (
	"os"
	"syscall"
)

func tryInstanceLock(path string) (func(), error) {
	const errorSharingViolation = syscall.Errno(32)
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	// The system releases this exclusive handle even when the process ends
	// abruptly. The file stays in place for the next launch to open.
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err == errorSharingViolation {
		return nil, nil
	}
	if err != nil {
		return nil, &os.PathError{Op: "lock", Path: path, Err: err}
	}
	return func() { _ = syscall.CloseHandle(handle) }, nil
}
