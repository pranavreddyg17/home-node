package install

import (
	"context"
	"crypto/ed25519"
	"errors"
	"testing"
)

// Rejected requests must never report completed storage or activation authority,
// including when no host has been opened yet.
func TestChannelProvisioningRefusesUntrustedRequestsBeforeHostAccess(t *testing.T) {
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
			result, err := engine.ProvisionGuestStorageChannels(context.Background(), request.key, request.floor)
			if !errors.Is(err, ErrPlan) || result != (GuestStorageChannelProvisioningResult{}) {
				t.Fatal("untrusted request produced authority", result, err)
			}
		})
	}
	result, err := engine.ProvisionGuestStorageChannels(context.Background(), make(ed25519.PublicKey, ed25519.PublicKeySize), 1)
	if !errors.Is(err, ErrAccounts) || result != (GuestStorageChannelProvisioningResult{}) {
		t.Fatal("absent host produced authority", result, err)
	}
}

func TestChannelProvisioningCancellationPrecedesTrustAndHostAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var engine *Engine
	result, err := engine.ProvisionGuestStorageChannels(ctx, nil, 0)
	if !errors.Is(err, context.Canceled) || result != (GuestStorageChannelProvisioningResult{}) {
		t.Fatal("cancelled request produced authority", result, err)
	}
}
