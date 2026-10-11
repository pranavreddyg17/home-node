package install

import (
	"context"
	"errors"
	"testing"
)

func TestGatewayRemovalAPIRejectsFixtureHostAndCancellation(t *testing.T) {
	host, journal := roots(t)
	engine := openEngine(t, host, journal)
	defer engine.Close()
	if err := engine.RemoveGatewayAccount(context.Background()); !errors.Is(err, ErrAccounts) {
		t.Fatal("fixture host mutation admitted", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := engine.RemoveGatewayAccount(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
}
