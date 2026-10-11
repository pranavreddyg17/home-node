package install

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestGatewayRemovalIntentPersistsWithoutAccountMutation(t *testing.T) {
	host, journal := roots(t)
	e := openEngine(t, host, journal)
	base := newAccountFixture()
	ctx := context.Background()
	backend := fixtureGatewayProvisioner{base}
	if _, err := e.provisionAccounts(ctx, base); err != nil {
		t.Fatal(err)
	}
	if _, err := e.prepareGatewayAccount(ctx, base); err != nil {
		t.Fatal(err)
	}
	if _, err := e.provisionGatewayAccount(ctx, backend); err != nil {
		t.Fatal(err)
	}
	plan, err := e.prepareGatewayAccountRemoval(ctx, backend)
	if err != nil || base.commands != 9 {
		t.Fatal("intent mutated accounts", err)
	}
	info, err := os.Lstat(filepath.Join(journal, "gateway-account-removal.json"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal(info, err)
	}
	e.Close()
	e = openEngine(t, host, journal)
	defer e.Close()
	replay, err := e.prepareGatewayAccountRemoval(ctx, backend)
	if err != nil || !reflect.DeepEqual(plan, replay) || base.commands != 9 {
		t.Fatal("intent replay differs", replay, err)
	}
	data, err := os.ReadFile(filepath.Join(journal, "gateway-account-removal.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(journal, "gateway-account-removal.json"), append(data, []byte("\n{}")...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.prepareGatewayAccountRemoval(ctx, backend); err == nil || base.commands != 9 {
		t.Fatal("substituted removal intent accepted", err)
	}
}

func TestGatewayRemovalLoaderRefusesCommandAndIdentitySubstitution(t *testing.T) {
	host, journal := roots(t)
	e := openEngine(t, host, journal)
	defer e.Close()
	base := newAccountFixture()
	ctx := context.Background()
	backend := fixtureGatewayProvisioner{base}
	if _, err := e.provisionAccounts(ctx, base); err != nil {
		t.Fatal(err)
	}
	if _, err := e.prepareGatewayAccount(ctx, base); err != nil {
		t.Fatal(err)
	}
	if _, err := e.provisionGatewayAccount(ctx, backend); err != nil {
		t.Fatal(err)
	}
	if _, err := e.prepareGatewayAccountRemoval(ctx, backend); err != nil {
		t.Fatal(err)
	}
	owned, err := e.loadAccountJournal()
	if err != nil {
		t.Fatal(err)
	}
	original, err := e.loadGatewayRemovalIntent(owned)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"command", "identity", "owner", "premature ready", "completed overflow"} {
		changed := original
		switch scenario {
		case "command":
			changed.Plan.Commands = append([]AccountCommand(nil), original.Plan.Commands...)
			changed.Plan.Commands[0].Program = "/bin/sh"
		case "identity":
			changed.Plan.Identity.UID++
		case "owner":
			changed.Plan.OwnerID = strings.Repeat("b", 32)
		case "premature ready":
			changed.Ready = true
		case "completed overflow":
			changed.Completed = 5
		}
		data, err := json.Marshal(changed)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(journal, "gateway-account-removal.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := e.loadGatewayRemovalIntent(owned); err == nil {
			t.Fatal("substituted removal intent loaded", scenario)
		}
	}
}
