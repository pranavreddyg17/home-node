package install

import (
	"context"
	"crypto/ed25519"
	"errors"
	"testing"
)

// Rejected requests must never report completed storage or activation authority,
// including when no host has been opened yet.
func TestEmptyVolumeMigrationRefusesUntrustedRequestsBeforeHostAccess(t *testing.T) {
	var engine *Engine
	for _, request := range []struct {
		name  string
		key   ed25519.PublicKey
		floor int64
	}{
		{"missing-key", nil, 1},
		{"short-key", make(ed25519.PublicKey, ed25519.PublicKeySize-1), 1},
		{"oversized-key", make(ed25519.PublicKey, ed25519.PublicKeySize+1), 1},
		{"missing-floor", make(ed25519.PublicKey, ed25519.PublicKeySize), 0},
		{"negative-floor", make(ed25519.PublicKey, ed25519.PublicKeySize), -1},
	} {
		t.Run(request.name, func(t *testing.T) {
			result, err := engine.MigrateEmptyGuestVolumeStorage(context.Background(), request.key, request.floor)
			if !errors.Is(err, ErrPlan) || result != (EmptyVolumeStorageMigrationResult{}) {
				t.Fatal("untrusted request produced authority", result, err)
			}
		})
	}
	result, err := engine.MigrateEmptyGuestVolumeStorage(context.Background(), make(ed25519.PublicKey, ed25519.PublicKeySize), 1)
	if !errors.Is(err, ErrAccounts) || result != (EmptyVolumeStorageMigrationResult{}) {
		t.Fatal("absent host produced authority", result, err)
	}
}

func TestEmptyVolumeMigrationCancellationPrecedesTrustAndHostAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var engine *Engine
	result, err := engine.MigrateEmptyGuestVolumeStorage(ctx, nil, 0)
	if !errors.Is(err, context.Canceled) || result != (EmptyVolumeStorageMigrationResult{}) {
		t.Fatal("cancelled request produced authority", result, err)
	}
}
