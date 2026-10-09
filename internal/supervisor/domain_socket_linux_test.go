//go:build linux

package supervisor

import (
	"context"
	"errors"
	"testing"
)

func TestReservedDomainSocketAdmission(t *testing.T) {
	var manager *Manager
	if err := manager.verifyReservedDomainSocket(context.Background(), Domain{}, 1); !errors.Is(err, ErrPolicy) {
		t.Fatalf("missing manager admitted: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := manager.verifyReservedDomainSocket(ctx, Domain{}, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}
