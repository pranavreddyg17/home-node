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
	plan := recoveryInstallPlan{Version: 1, SnapshotID: strings.Repeat("c", 64), ManifestSHA256: strings.Repeat("d", 64), OwnerID: state.Random(), RecoveryEpoch: 3, Disks: []RecoveryInstallDisk{disk}}
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
	reopened, err := loadRecoveryInstallPlan(context.Background(), root)
	if err != nil || reopened.SnapshotID != saved.SnapshotID || reopened.OwnerID != saved.OwnerID || reopened.RecoveryEpoch != saved.RecoveryEpoch || len(reopened.Disks) != 1 || reopened.Disks[0] != disk {
		t.Fatal("reopening changed durable identity", err)
	}
	for _, invalid := range []string{
		strings.Replace(string(data), `"version":1`, `"version":1,"Version":1`, 1),
		strings.Replace(string(data), `"workload":"files"`, `"workload":"files","workload":"ai"`, 1),
		strings.Replace(string(data), `"sourceName":"files.raw"`, `"sourceName":"../files.raw"`, 1),
		strings.Replace(string(data), `"bytes":16777216`, `"bytes":null`, 1),
		string(data) + `{}`,
	} {
		if plan, decodeErr := decodeRecoveryInstallPlan([]byte(invalid)); decodeErr == nil || plan.Disks != nil {
			t.Fatal("ambiguous journal returned plan", invalid, decodeErr)
		}
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
	if reopened, err := loadRecoveryInstallPlan(ctx, root); !errors.Is(err, context.Canceled) || reopened.Disks != nil {
		t.Fatal("cancelled reopening returned plan", err)
	}
	if err = os.Chmod(path+"/recovery-install.json", 0644); err != nil {
		t.Fatal(err)
	}
	if reopened, err := loadRecoveryInstallPlan(context.Background(), root); !errors.Is(err, ErrManifest) || reopened.Disks != nil {
		t.Fatal("public journal returned plan", err)
	}
}

func TestRecoveryInstallPlanRequalificationRequiresFilesystem(t *testing.T) {
	root, manifest, policy, _ := recoverySet(t)
	entry := manifest.Files[1]
	digest, err := recoveryManifestDigest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	plan := recoveryInstallPlan{Version: 1, SnapshotID: strings.Repeat("c", 64), ManifestSHA256: digest, OwnerID: state.Random(), RecoveryEpoch: 3, Disks: []RecoveryInstallDisk{{Workload: entry.Workload, SourceName: entry.Name, Bytes: entry.Bytes, SourceSHA256: entry.SHA256, ImageSHA256: entry.ImageSHA256, InstanceID: state.Random()}}}
	constructed, err := newRecoveryInstallPlan(context.Background(), root, plan.SnapshotID, manifest, plan.Disks)
	if err != nil || constructed.RecoveryEpoch != 3 || constructed.OwnerID == "" || constructed.ManifestSHA256 != digest {
		t.Fatal("replacement identity plan differs", constructed, err)
	}
	metadata, err := recoverySourceMetadata(context.Background(), root)
	if err != nil || constructed.OwnerID == metadata.OwnerID {
		t.Fatal("historical owner reused", err)
	}
	if err := requalifyRecoveryInstallPlan(context.Background(), root, plan, manifest, policy); err == nil {
		t.Fatal("journaled bytes bypassed filesystem qualification")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := requalifyRecoveryInstallPlan(ctx, root, plan, manifest, policy); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
}
