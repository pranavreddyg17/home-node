//go:build linux || darwin

package backup

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestJobStagingOperationRetainsParentLeaseUntilClose(t *testing.T) {
	parent := t.TempDir()
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	staging, err := OpenJobStaging(context.Background(), parent, state.Random())
	if err != nil {
		t.Fatal(err)
	}
	release, err := staging.holdOperation(context.Background())
	if err != nil {
		staging.Close()
		t.Fatal(err)
	}
	if other, err := staging.holdOperation(context.Background()); !errors.Is(err, ErrMaintenanceRunner) || other != nil {
		release()
		staging.Close()
		t.Fatal("overlapping staging operation admitted", err)
	}
	closeStarted := make(chan struct{})
	closeDone := make(chan error, 1)
	go func() { close(closeStarted); closeDone <- staging.Close() }()
	<-closeStarted
	// While the operation is held, neither root nor parent kernel lease closes.
	writeErr := staging.Root().WriteFile("operation-evidence", []byte("retained"), 0600)
	_, overlapErr := OpenJobStaging(context.Background(), parent, state.Random())
	release()
	closeErr := <-closeDone
	if writeErr != nil || overlapErr == nil || closeErr != nil {
		t.Fatal("staging lifetime/lease mismatch", writeErr, overlapErr, closeErr)
	}
	if next, err := staging.holdOperation(context.Background()); !errors.Is(err, ErrManifest) || next != nil {
		t.Fatal("closed staging accepted operation", err)
	}
	newStaging, err := OpenJobStaging(context.Background(), parent, state.Random())
	if err != nil {
		t.Fatal("closed staging retained parent lease", err)
	}
	if err = newStaging.Close(); err != nil {
		t.Fatal(err)
	}
}
