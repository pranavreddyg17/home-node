package install

import (
	"context"
	"os"
	"testing"
)

func readyAccountIntent(t *testing.T, e *Engine, a Accounts, ready bool) {
	t.Helper()
	j := accountJournal{Version: 1, OwnerID: "0123456789abcdef0123456789abcdef", Accounts: a, Completed: 5, Ready: ready}
	j.Digest = accountIntentDigest(j)
	if err := e.saveAccountJournal(j); err != nil {
		t.Fatal(err)
	}
}
func TestConfigureRequiresOwnedVerifiedAccounts(t *testing.T) {
	for _, which := range []string{"missing", "unfinished", "changed", "inspection"} {
		t.Run(which, func(t *testing.T) {
			c, _, _, now := configurationFixture(t)
			host, jr := roots(t)
			e := openEngine(t, host, jr)
			defer e.Close()
			if which != "missing" {
				readyAccountIntent(t, e, c.Accounts, which != "unfinished")
			}
			called := false
			_, err := e.configure(context.Background(), c, now, func(context.Context) (Accounts, Capacity, error) {
				called = true
				if which == "inspection" {
					return Accounts{}, Capacity{}, ErrAccounts
				}
				a := c.Accounts
				a.ControllerUID--
				return a, c.Capacity, nil
			})
			if err == nil {
				t.Fatal("invalid admission accepted")
			}
			if (which == "missing" || which == "unfinished") && called {
				t.Fatal("observed before ownership admission")
			}
			if _, err := e.load(); !os.IsNotExist(err) {
				t.Fatal("configuration intent written", err)
			}
		})
	}
}
func TestRootConfigureBindsObservedIdentityAndCapacity(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-only temporary fixture")
	}
	c, _, _, now := configurationFixture(t)
	host, jr := roots(t)
	e := openEngine(t, host, jr)
	defer e.Close()
	a, capacity := c.Accounts, c.Capacity
	readyAccountIntent(t, e, a, true)
	c.Accounts = Accounts{}
	c.Capacity = Capacity{}
	c.Policy.ControllerUID = 0
	c.Policy.TransferUID = 0
	observe := func(context.Context) (Accounts, Capacity, error) { return a, capacity, nil }
	p, err := e.configure(context.Background(), c, now, observe)
	if err != nil {
		t.Fatal(err)
	}
	if p.Accounts != a || p.ProvidedCapacity != capacity || p.RuntimePolicy.ControllerUID != a.ControllerUID {
		t.Fatal("unbound configuration")
	}
	if _, err = e.configure(context.Background(), c, now, observe); err != nil {
		t.Fatal("replay", err)
	}
	if err = e.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
}
