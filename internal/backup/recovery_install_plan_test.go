package backup

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestRecoveryInstallPlanPersistsFreshIdentityBeforeEffects(t *testing.T) {
	path := t.TempDir()
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	disk := RecoveryInstallDisk{Workload: "files", SourceName: "files.raw", Bytes: 16 << 20, SourceSHA256: strings.Repeat("a", 64), ImageSHA256: strings.Repeat("b", 64), InstanceID: state.Random()}
	plan := recoveryInstallPlan{Version: 1, SnapshotID: strings.Repeat("c", 64), Disks: []RecoveryInstallDisk{disk}}
	if err = createRecoveryInstallPlan(context.Background(), root, plan); err != nil {
		t.Fatal(err)
	}
	data, err := root.ReadFile("recovery-install.json")
	if err != nil {
		t.Fatal(err)
	}
	var saved recoveryInstallPlan
	if json.Unmarshal(data, &saved) != nil || saved.validate() != nil || len(saved.Disks) != 1 || saved.Disks[0] != disk {
		t.Fatal("journal changed target identity")
	}
	plan.Disks[0].InstanceID = state.Random()
	if err = createRecoveryInstallPlan(context.Background(), root, plan); !errors.Is(err, os.ErrExist) {
		t.Fatal("occupied journal adopted", err)
	}
	retained, _ := root.ReadFile("recovery-install.json")
	if string(retained) != string(data) {
		t.Fatal("occupied journal changed")
	}
	plan.Disks = append(plan.Disks, plan.Disks[0])
	if plan.validate() == nil {
		t.Fatal("duplicate workload/identity accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = createRecoveryInstallPlan(ctx, root, plan); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
}
