//go:build darwin || linux

package desktop

import (
	"os"
	"syscall"
)

func tryInstanceLock(path string) (func(), error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err != syscall.EINTR {
			break
		}
	}
	if err == nil {
		// Keep the same inode available for later launches; closing releases the lock.
		return func() { _ = file.Close() }, nil
	}
	_ = file.Close()
	if err == syscall.EWOULDBLOCK {
		return nil, nil
	}
	return nil, &os.PathError{Op: "flock", Path: path, Err: err}
}
