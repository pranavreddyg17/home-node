//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestNativeGuestUIDLocalAccountObservation(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_GUEST_UID_ACCOUNTS_INTEGRATION") != "1" {
		t.Skip("explicit disposable Linux root fixture")
	}
	pool := GuestUIDPool{First: 200000, Last: 200015}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ObserveLocalGuestUIDConflicts(ctx, pool); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled observation ignored", err)
	}
	before := make(map[string]os.FileInfo)
	for _, path := range []string{"/etc", "/etc/passwd", "/etc/subuid"} {
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		before[path] = info
	}
	observed, err := ObserveLocalGuestUIDConflicts(context.Background(), pool)
	if err != nil || observed.First != pool.First || observed.Last != pool.Last {
		t.Fatal("native account observation", observed, err)
	}
	processes, err := ObserveGuestUIDProcessConflicts(context.Background(), observed)
	if err != nil || processes.First != pool.First || processes.Last != pool.Last {
		t.Fatal("native process UID observation", processes, err)
	}
	if _, err = ObserveGuestUIDProcessConflicts(ctx, pool); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled process observation ignored", err)
	}
	if pool.Blocked != nil {
		t.Fatal("input policy mutated")
	}
	for path, original := range before {
		current, err := os.Lstat(path)
		if err != nil || !os.SameFile(original, current) || original.Mode() != current.Mode() || original.Size() != current.Size() {
			t.Fatal("observation changed account path", path, err)
		}
	}
}
