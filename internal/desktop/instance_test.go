package desktop

import (
	"path/filepath"
	"testing"
)

func TestInstanceLockHeldUntilRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instance.lock")
	func() {
		first, err := tryInstanceLock(path)
		if first == nil || err != nil {
			t.Fatalf("first launch could not acquire the lock: %v", err)
		}
		defer first()
		second, err := tryInstanceLock(path)
		if second != nil {
			second()
			t.Fatal("a second launch acquired the lock")
		}
		if err != nil {
			t.Fatalf("a held lock must be treated as an existing instance: %v", err)
		}
	}()
	next, err := tryInstanceLock(path)
	if next == nil || err != nil {
		t.Fatalf("launch after release could not acquire the lock: %v", err)
	}
	next()
}
