package backup

import (
	"context"
	"errors"
	"testing"
)

func TestRecoveryInstallInventoryRefusesChecksummedNonFilesystem(t *testing.T) {
	root, manifest, policy, _ := recoverySet(t)
	if err := ValidateRecoverySet(context.Background(), root, manifest, policy); err != nil {
		t.Fatal(err)
	}
	inventory, err := RecoveryInstallInventory(context.Background(), root, manifest, policy)
	if err == nil || inventory != nil {
		t.Fatal("metadata validity became install inventory", inventory, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if inventory, err := RecoveryInstallInventory(ctx, root, manifest, policy); !errors.Is(err, context.Canceled) || inventory != nil {
		t.Fatal("cancelled qualification returned inventory", inventory, err)
	}
}
