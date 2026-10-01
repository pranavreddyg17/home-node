//go:build linux || darwin

package backup

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestJobStagingLeaseExcludesOverlapAndPreservesData(t *testing.T) {
	parent := t.TempDir()
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	job := state.Random()
	first, err := OpenJobStaging(context.Background(), parent, job)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if err = first.Root().WriteFile("retained", []byte("protected staged data"), 0600); err != nil {
		t.Fatal(err)
	}
	if second, err := OpenJobStaging(context.Background(), parent, state.Random()); !errors.Is(err, ErrMaintenanceRunner) || second != nil {
		t.Fatal("overlapping stage admitted", err)
	}
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	if err = first.Close(); err != nil {
		t.Fatal("lease close not idempotent", err)
	}
	if reopened, err := OpenJobStaging(context.Background(), parent, job); err == nil || reopened != nil {
		t.Fatal("existing job adopted")
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	data, err := root.ReadFile(job + "/retained")
	if err != nil || string(data) != "protected staged data" {
		t.Fatal("closing or replay erased staging", err)
	}
	next, err := OpenJobStaging(context.Background(), parent, state.Random())
	if err != nil {
		t.Fatal("new job could not acquire released lease", err)
	}
	defer next.Close()
}
func TestJobStagingRefusesUnsafeParentAndJob(t *testing.T) {
	parent := t.TempDir()
	if err := os.Chmod(parent, 0755); err != nil {
		t.Fatal(err)
	}
	if lease, err := OpenJobStaging(context.Background(), parent, state.Random()); err == nil || lease != nil {
		t.Fatal("public staging parent accepted")
	}
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	for _, job := range []string{"../foreign", "short", ""} {
		if lease, err := OpenJobStaging(context.Background(), parent, job); err == nil || lease != nil {
			t.Fatal("unsafe job accepted")
		}
	}
}
