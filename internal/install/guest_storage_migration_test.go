package install

import (
	"context"
	"errors"
	"testing"
)

func TestGuestStorageMigrationRejectsMissingConsumerAndCancellation(t *testing.T) {
	e := &Engine{}
	if err := e.withQualifiedGuestStorageImagesMigrationLocked(context.Background(), nil, nil, nil); !errors.Is(err, ErrPlan) {
		t.Fatal("missing image migration consumer admitted", err)
	}
	if err := e.withQualifiedGuestStorageMigrationLocked(context.Background(), nil, nil, nil); !errors.Is(err, ErrPlan) {
		t.Fatal("missing publisher admitted", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.withQualifiedGuestStorageImagesMigrationLocked(ctx, nil, nil, func(context.Context, guestStorageImagesIntent, func(context.Context) error) error {
		t.Fatal("canceled image migration consumer called")
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatal("image migration cancellation lost", err)
	}
	called := false
	if err := e.withQualifiedGuestStorageMigrationLocked(ctx, nil, nil, func(context.Context, GuestStorageProvisioningPlan, func(context.Context) error) error {
		called = true
		return nil
	}); !errors.Is(err, context.Canceled) || called {
		t.Fatal("canceled migration admitted", called, err)
	}
}
