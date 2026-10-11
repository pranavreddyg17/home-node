package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGatewayTLSRetirementAPIRefusesFixtureHostAndCanceledAuthority(t *testing.T) {
	host, jr := roots(t)
	e := openEngine(t, host, jr)
	defer e.Close()
	if err := e.RetireGatewayTLS(context.Background()); !errors.Is(err, ErrAccounts) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.RetireGatewayTLS(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(jr, "gateway-tls-retirement.json")); !os.IsNotExist(err) {
		t.Fatal("refused operation published intent", err)
	}
}
