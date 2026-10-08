package install

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestGuestIdentityApplicationRefusesFixtureHostBeforeEffects(t *testing.T) {
	host, journal := roots(t)
	e := openEngine(t, host, journal)
	defer e.Close()
	before, err := os.ReadDir(journal)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := e.ApplyGuestIdentityConfiguration(context.Background())
	if !errors.Is(err, ErrAccounts) || preview != (GuestIdentityConfigurationPreview{}) {
		t.Fatal("fixture host admitted for native identity application", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	preview, err = e.ApplyGuestIdentityConfiguration(ctx)
	if !errors.Is(err, context.Canceled) || preview != (GuestIdentityConfigurationPreview{}) {
		t.Fatal("canceled application admitted", err)
	}
	after, err := os.ReadDir(journal)
	if err != nil || len(before) != len(after) {
		t.Fatal("refused application changed journal", err)
	}
	for i := range before {
		if before[i].Name() != after[i].Name() {
			t.Fatal("refused application replaced journal entries")
		}
	}
}
