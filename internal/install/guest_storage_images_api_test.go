package install

import (
	"context"
	"errors"
	"testing"
)

func TestGuestImageStorageMigrationRequiresReleaseAuthority(t *testing.T) {
	e := &Engine{}
	if result, err := e.MigrateGuestImageStorage(context.Background(), nil, 0); !errors.Is(err, ErrPlan) || result != (ImageStorageMigrationResult{}) {
		t.Fatal("missing release authority admitted", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result, err := e.MigrateGuestImageStorage(ctx, nil, 0); !errors.Is(err, context.Canceled) || result != (ImageStorageMigrationResult{}) {
		t.Fatal("cancelled migration returned authority", result, err)
	}
}
