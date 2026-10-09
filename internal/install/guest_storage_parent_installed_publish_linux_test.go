//go:build linux

package install

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestGuestStorageInstalledParentPublisherRefusesMissingAuthority(t *testing.T) {
	e := &Engine{}
	if err := e.publishInstalledGuestStorageImageParentLocked(context.Background(), nil, 0, nil, nil); !errors.Is(err, ErrPlan) {
		t.Fatal("missing release/runtime authority admitted", err)
	}
	if err := e.withGuestStorageParentAuthorityLocked(context.Background(), nil); !errors.Is(err, ErrPlan) {
		t.Fatal("missing parent consumer admitted", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.publishInstalledGuestStorageImageParentLocked(ctx, nil, 0, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("publisher cancellation lost", err)
	}
	if err := e.withGuestStorageParentAuthorityLocked(ctx, func(context.Context, guestStorageParentAuthority, *os.Root, *os.File, func(context.Context) error) error {
		t.Fatal("cancelled authority consumer called")
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatal("authority cancellation lost", err)
	}
}
