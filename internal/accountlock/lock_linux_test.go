//go:build linux

package accountlock

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestRootLockRefusesParentPermissionDrift(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("owned disposable root fixture")
	}
	path := t.TempDir()
	directory, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	defer os.Chmod(path, 0700)
	err = With(context.Background(), directory, func(ctx context.Context, check func() error) error {
		if err := os.Chmod(path, 0777); err != nil {
			return err
		}
		if err := check(); !errors.Is(err, ErrConflict) {
			t.Fatal("unprotected parent admitted", err)
		}
		return nil
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatal("drifted scope succeeded", err)
	}
}
