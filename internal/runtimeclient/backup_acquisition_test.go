package runtimeclient

import (
	"context"
	"errors"
	"github.com/pranavreddyg17/home-node/internal/state"
	"reflect"
	"testing"
)

type acquisitionFixture struct {
	steps                    []string
	fail                     string
	root, job, token, device string
}

func (f *acquisitionFixture) ConfirmFreezing(_ context.Context, token, job, device string) error {
	f.steps = append(f.steps, "preflight")
	if token != f.token || job != f.job || device != f.device {
		return ErrMaintenance
	}
	if f.fail == "preflight" {
		return ErrMaintenance
	}
	return nil
}
func (f *acquisitionFixture) BeginRuntimeMaintenanceForJob(_ context.Context, job string) (string, error) {
	f.steps = append(f.steps, "acquire")
	if job != f.job || f.fail == "acquire" {
		return "", ErrMaintenance
	}
	return f.root, nil
}
func (f *acquisitionFixture) AttachRuntimeRoot(_ context.Context, token, device, root string) error {
	f.steps = append(f.steps, "attach")
	if token != f.token || device != f.device || root != f.root || f.fail == "attach" {
		return ErrMaintenance
	}
	return nil
}
func (f *acquisitionFixture) EndRuntimeMaintenance(context.Context, string) error {
	f.steps = append(f.steps, "release")
	return ErrMaintenance
}
func TestBackupRuntimeAcquisitionRetainsUncertainAttachment(t *testing.T) {
	for _, scenario := range []string{"valid", "preflight", "acquire", "attach"} {
		t.Run(scenario, func(t *testing.T) {
			f := &acquisitionFixture{fail: scenario, root: state.Random(), job: state.Random(), token: state.Random(), device: state.Random()}
			root, err := AcquireOwnedBackupRuntime(context.Background(), f, f, f.token, f.job, f.device)
			want := []string{"preflight", "acquire", "attach"}
			if scenario == "preflight" {
				want = want[:1]
			}
			if scenario == "acquire" {
				want = want[:2]
			}
			if !reflect.DeepEqual(f.steps, want) {
				t.Fatal("unexpected authority effects", f.steps)
			}
			if (err == nil) != (scenario == "valid") {
				t.Fatal("unexpected result", err)
			}
			if scenario == "valid" || scenario == "attach" {
				if root != f.root {
					t.Fatal("known authority lost")
				}
			} else if root != "" {
				t.Fatal("unknown authority exposed")
			}
		})
	}
	f := &acquisitionFixture{root: state.Random(), job: state.Random(), token: state.Random(), device: state.Random()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if root, err := AcquireOwnedBackupRuntime(ctx, f, f, f.token, f.job, f.device); !errors.Is(err, context.Canceled) || root != "" || len(f.steps) != 0 {
		t.Fatal("cancelled acquisition had effects", root, err, f.steps)
	}
}
