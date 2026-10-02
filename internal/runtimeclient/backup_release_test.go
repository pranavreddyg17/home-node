package runtimeclient

import (
	"context"
	"github.com/pranavreddyg17/home-node/internal/state"
	"reflect"
	"testing"
)

type releaseFixture struct {
	steps                     []string
	fail, token, device, root string
}

func (f *releaseFixture) ConfirmRuntimeRelease(_ context.Context, token, device, root string) error {
	f.steps = append(f.steps, "preflight")
	if token != f.token || device != f.device || root != f.root || f.fail == "preflight" {
		return ErrMaintenance
	}
	return nil
}
func (f *releaseFixture) EndRuntimeMaintenance(_ context.Context, root string) error {
	f.steps = append(f.steps, "release")
	if root != f.root || f.fail == "release" {
		return ErrMaintenance
	}
	return nil
}
func (f *releaseFixture) RecordRuntimeReleased(_ context.Context, token, device, root string) error {
	f.steps = append(f.steps, "record")
	if token != f.token || device != f.device || root != f.root || f.fail == "record" {
		return ErrMaintenance
	}
	return nil
}
func (f *releaseFixture) BeginRuntimeMaintenanceForJob(context.Context, string) (string, error) {
	f.steps = append(f.steps, "unexpected-acquisition")
	return "", ErrMaintenance
}
func TestOwnedRuntimeReleaseRequiresStoppedJobAndAcknowledgement(t *testing.T) {
	for _, scenario := range []string{"valid", "preflight", "release", "record"} {
		t.Run(scenario, func(t *testing.T) {
			f := &releaseFixture{fail: scenario, token: state.Random(), device: state.Random(), root: state.Random()}
			err := ReleaseOwnedBackupRuntime(context.Background(), f, f, f.token, f.device, f.root)
			want := []string{"preflight", "release", "record"}
			if scenario == "preflight" {
				want = want[:1]
			}
			if scenario == "release" {
				want = want[:2]
			}
			if !reflect.DeepEqual(f.steps, want) || (err == nil) != (scenario == "valid") {
				t.Fatal("unexpected release effects", f.steps, err)
			}
		})
	}
}
