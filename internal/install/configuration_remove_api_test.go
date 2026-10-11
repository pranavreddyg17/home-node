package install

import (
	"context"
	"errors"
	"testing"
)

func TestConfigurationRemovalAPIRejectsFixtureHostAndCancellation(t *testing.T) {
	e, _, _ := configurationRemoveFixture(t)
	defer e.Close()
	if err := e.RemoveConfiguration(context.Background()); !errors.Is(err, ErrAccounts) {
		t.Fatal("fixture host admitted to native manager", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.RemoveConfiguration(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled removal admitted", err)
	}
}
