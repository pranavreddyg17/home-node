package install

import (
	"context"
	"strings"
	"testing"
)

func TestGatewayRemovalPlanRequiresOwnedReadyIdentityWithoutMutation(t *testing.T) {
	host, journal := roots(t)
	engine := openEngine(t, host, journal)
	defer engine.Close()
	base := newAccountFixture()
	ctx := context.Background()
	backend := fixtureGatewayProvisioner{base}
	if _, err := engine.provisionAccounts(ctx, base); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.planGatewayAccountRemoval(ctx, backend); err == nil {
		t.Fatal("unowned gateway removal planned")
	}
	if _, err := engine.prepareGatewayAccount(ctx, base); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.planGatewayAccountRemoval(ctx, backend); err == nil {
		t.Fatal("partial gateway removal planned")
	}
	identity, err := engine.provisionGatewayAccount(ctx, backend)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := engine.planGatewayAccountRemoval(ctx, backend)
	if err != nil || plan.Identity != identity || len(plan.Commands) != 4 || len(plan.Pending) == 0 || base.commands != 9 {
		t.Fatal("removal planning mutation or ownership failure", plan, err)
	}
	for _, command := range plan.Commands {
		for _, argument := range command.Arguments {
			if argument == "--remove" || argument == "-r" {
				t.Fatal("removal would erase service data")
			}
		}
	}
	base.s.passwd = []byte(strings.Replace(string(base.s.passwd), "HomeNode install ", "Foreign install ", 1))
	if _, err := engine.planGatewayAccountRemoval(ctx, backend); err == nil || base.commands != 9 {
		t.Fatal("foreign base ownership accepted", err)
	}
}
