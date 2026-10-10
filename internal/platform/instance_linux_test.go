package platform

import (
	"path/filepath"
	"testing"
)

func TestInstanceLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tracker.lock")
	first, err := LockInstance(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if second, err := LockInstance(path); err == nil {
		second.Close()
		t.Fatal("second instance acquired lock")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	third, err := LockInstance(path)
	if err != nil {
		t.Fatalf("lock not released: %v", err)
	}
	third.Close()
}
