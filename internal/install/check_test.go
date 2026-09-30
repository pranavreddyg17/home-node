package install

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRootPreparedInspectionRejectsChangedAndIncompleteState(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-only temporary fixture")
	}
	for _, which := range []string{"valid", "incomplete", "changed-environment", "expired", "wrong-account", "missing-floor", "changed-image"} {
		t.Run(which, func(t *testing.T) {
			e, c, host, jr, source, now := imagePlacementFixture(t)
			defer e.Close()
			readyAccountIntent(t, e, c.Accounts, true)
			if which != "incomplete" {
				if err := e.placeImages(context.Background(), source, c.Publisher, c.MinimumCatalogVersion, now); err != nil {
					t.Fatal(err)
				}
			}
			switch which {
			case "changed-environment":
				if err := os.WriteFile(filepath.Join(host, "etc/homenode/services.env"), []byte("TAILNET_IP=100.100.1.9\n"), 0644); err != nil {
					t.Fatal(err)
				}
			case "expired":
				now = now.Add(2 * time.Hour)
			case "wrong-account":
				a := c.Accounts
				a.ControllerUID--
				readyAccountIntent(t, e, a, true)
			case "missing-floor":
				if err := os.Remove(filepath.Join(host, "etc/homenode/catalog-floor")); err != nil {
					t.Fatal(err)
				}
			case "changed-image":
				entries, err := os.ReadDir(filepath.Join(host, "var/lib/homenode/images"))
				if err != nil {
					t.Fatal(err)
				}
				if err = os.Chmod(filepath.Join(host, "var/lib/homenode/images", entries[0].Name()), 0640); err != nil {
					t.Fatal(err)
				}
			}
			before := map[string][]byte{}
			entries, err := os.ReadDir(jr)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				data, err := os.ReadFile(filepath.Join(jr, entry.Name()))
				if err != nil {
					t.Fatal(err)
				}
				before[entry.Name()] = data
			}
			result, err := e.checkPrepared(context.Background(), now)
			if which == "valid" {
				if err != nil {
					t.Fatal(err)
				}
				if !result.ArtifactsVerified || result.AccountsVerified || result.HTTPSIdentityVerified || result.Network != c.Network || result.CatalogFloor != 4 || result.CatalogVersion != 4 || len(result.Pending) == 0 {
					t.Fatal("incorrect inspection claims", result)
				}
			} else if err == nil {
				t.Fatal("invalid prepared state admitted", which)
			}
			for name, data := range before {
				after, err := os.ReadFile(filepath.Join(jr, name))
				if err != nil || !bytes.Equal(data, after) {
					t.Fatal("inspection changed journal", name, err)
				}
			}
		})
	}
}
func TestRootTLSAccessUsesOnlyControllerPrivateGroup(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-only temporary fixture")
	}
	e, c, host, _, _, _ := imagePlacementFixture(t)
	defer e.Close()
	for _, name := range []string{"server.crt", "server.key"} {
		if err := os.WriteFile(filepath.Join(host, "etc/homenode/tls", name), []byte("fixture permissions only"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cert, key := filepath.Join(host, "etc/homenode/tls/server.crt"), filepath.Join(host, "etc/homenode/tls/server.key")
	if err := os.Chmod(cert, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(key, 0, c.Accounts.ControllerGID); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(key, 0640); err != nil {
		t.Fatal(err)
	}
	if err := e.checkTLSAccess(c.Accounts); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(key, 0, c.Accounts.RuntimeGID); err != nil {
		t.Fatal(err)
	}
	if err := e.checkTLSAccess(c.Accounts); err == nil {
		t.Fatal("shared runtime group can read key")
	}
}
