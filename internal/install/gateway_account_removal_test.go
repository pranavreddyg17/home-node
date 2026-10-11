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

func TestGatewayRemovalRequiresRetainedPermissionOwnershipTransition(t *testing.T) {
	c, _, _, now := configurationFixture(t)
	identity := GatewayAccount{UID: 803, GID: 803, ProxyGID: 804}
	c.Gateway = &identity
	preview, err := ConfigurationPlan(c, now)
	if err != nil {
		t.Fatal(err)
	}
	records, _, err := planRecords(preview.Plan, 0)
	if err != nil {
		t.Fatal(err)
	}
	references := gatewayRemovalIdentityReferences(journal{Items: records}, identity)
	if len(references) != 1 || references[0] != "etc/homenode/tls" {
		t.Fatal("retained TLS permission not protected from identity reuse", references)
	}
	legacy := journal{Items: []record{{Path: "var/lib/homenode/control", UID: 800, GID: 800}, {Path: "etc/homenode/tls", UID: 0, GID: 800}}}
	if references := gatewayRemovalIdentityReferences(legacy, identity); len(references) != 0 {
		t.Fatal("unrelated identities block retirement", references)
	}
	alias := journal{Items: []record{{Path: "var/lib/homenode/control", UID: 803, GID: 800}, {Path: "etc/homenode/services.env", UID: 0, GID: 804}}}
	if references := gatewayRemovalIdentityReferences(alias, identity); len(references) != 2 {
		t.Fatal("retained UID or bridge GID omitted", references)
	}
}
