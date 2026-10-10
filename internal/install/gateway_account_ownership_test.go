package install

import (
	"context"
	"testing"
)

func TestOwnedBaseAccountsRequireCompletedGatewayJournal(t *testing.T) {
	host, journal := roots(t)
	engine := openEngine(t, host, journal)
	defer engine.Close()
	fixture := newAccountFixture()
	ctx := context.Background()
	if _, err := engine.provisionAccounts(ctx, fixture); err != nil {
		t.Fatal(err)
	}
	base, err := engine.loadAccountJournal()
	if err != nil {
		t.Fatal(err)
	}
	for step := 0; step < 5; step++ {
		matched, err := engine.ownedAccountStepMatches(fixture.s, base, step)
		if err != nil || !matched {
			t.Fatal("legacy ownership refused", step, err)
		}
	}
	if _, err := engine.prepareGatewayAccount(ctx, fixture); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.ownedAccountStepMatches(fixture.s, base, 4); err == nil {
		t.Fatal("pending gateway permitted operational ownership")
	}
	backend := fixtureGatewayProvisioner{fixture}
	if _, err := engine.provisionGatewayAccount(ctx, backend); err != nil {
		t.Fatal(err)
	}
	for step := 0; step < 5; step++ {
		matched, err := engine.ownedAccountStepMatches(fixture.s, base, step)
		if err != nil || !matched {
			t.Fatal("owned proxy membership refused", step, err)
		}
	}
	if _, err := engine.provisionAccounts(ctx, fixture); err != nil || fixture.commands != 9 {
		t.Fatal("base ready replay failed", err)
	}
	if _, err := engine.prepareMaintenanceAccount(ctx, fixture); err != nil {
		t.Fatal("gateway blocked maintenance intent", err)
	}
}
