//go:build linux

package supervisor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGuestUIDTaskObservationIncludesNonLeaderCredentials(t *testing.T) {
	directory := t.TempDir()
	for task, status := range map[string]string{"100": "Uid:\t0\t0\t0\t0\n", "101": "Uid:\t0\t200000\t0\t200001\n"} {
		path := filepath.Join(directory, "100", "task", task)
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "status"), []byte(status), 0600); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	pool := GuestUIDPool{First: 200000, Last: 200001, Blocked: make(map[uint32]bool)}
	count := 0
	if err = observeGuestUIDTaskConflicts(context.Background(), root, "100", pool, &count); err != nil {
		t.Fatal(err)
	}
	if count != 2 || !pool.Blocked[200000] || !pool.Blocked[200001] {
		t.Fatal("nonleader credentials missed", count, pool)
	}
	if err = root.Remove("100/task/101/status"); err != nil {
		t.Fatal(err)
	}
	// A task disappearing before status opening is skipped without inventing UID observations.
	count = 0
	if err = observeGuestUIDTaskConflicts(context.Background(), root, "100", pool, &count); err != nil {
		t.Fatal("disappeared task status refused", err)
	}
}
