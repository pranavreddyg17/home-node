package install

import (
	"context"
	"errors"
	"testing"
)

func TestConfigurationRetirementAPIRejectsFixtureHostAndCancellation(t *testing.T) {
	e, _, _ := configurationRemoveFixture(t)
	defer e.Close()
	if err := e.RetireConfigurationUnits(context.Background()); !errors.Is(err, ErrAccounts) {
		t.Fatal("fixture host admitted native reload", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.RetireConfigurationUnits(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
