package backup

import (
	"context"
	"os"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
)

// ValidateRecoverySet adds database authority/inventory checks to payload
// verification. It does not install disks, enroll clients or start workloads.
func ValidateRecoverySet(ctx context.Context, root *os.Root, manifest Manifest, policy RestorePolicy) error {
	if root == nil {
		return ErrManifest
	}
	info, err := root.Stat(".")
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return ErrManifest
	}
	if err := VerifyPayload(ctx, root, manifest, policy); err != nil {
		return err
	}
	snapshot, err := root.Open("snapshot.db")
	if err != nil {
		return ErrManifest
	}
	defer snapshot.Close()
	apps, err := state.ValidateRecoverySnapshot(ctx, snapshot)
	if err != nil {
		return ErrManifest
	}
	declared := map[string]bool{}
	for _, file := range manifest.Files {
		if file.Workload != "management" {
			declared[file.Workload] = true
		}
	}
	for _, app := range apps {
		if !declared[app.Workload] {
			return ErrManifest
		}
		delete(declared, app.Workload)
	}
	if len(declared) != 0 || manifest.Validate(policy, time.Now()) != nil {
		return ErrManifest
	}
	return nil
}
