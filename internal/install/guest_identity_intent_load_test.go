package install

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGuestIdentityIntentReloadRejectsUnownedOrAlteredRecords(t *testing.T) {
	for _, fault := range []string{"none", "foreign-owner", "proposal", "duplicate-key", "unknown-key", "trailing-data", "writable", "hardlink", "symlink", "canceled"} {
		t.Run(fault, func(t *testing.T) {
			host, journal := roots(t)
			e := openEngine(t, host, journal)
			defer e.Close()
			owner := strings.Repeat("a", 32)
			original := []byte("passwd: files systemd\ngroup: files\nshadow: files\n")
			proposal, err := planGuestIdentityNameServices(original)
			if err != nil {
				t.Fatal(err)
			}
			if err := e.commitGuestIdentityNameServices(context.Background(), owner, original, proposal); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(journal, "guest-identity-nss-intent.json")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch fault {
			case "foreign-owner":
				owner = strings.Repeat("b", 32)
			case "proposal":
				var record guestIdentityNameServiceIntent
				if err := json.Unmarshal(data, &record); err != nil {
					t.Fatal(err)
				}
				record.Proposal.Contents += "hosts: files\n"
				data, err = json.Marshal(record)
			case "duplicate-key":
				data = append([]byte(`{"version":1,`), data[1:]...)
			case "unknown-key":
				data = append([]byte(`{"unknown":true,`), data[1:]...)
			case "trailing-data":
				data = append(data, []byte("{}")...)
			case "writable":
				err = os.Chmod(path, 0666)
			case "hardlink":
				err = os.Link(path, path+".alias")
			case "symlink":
				err = os.Rename(path, path+".original")
				if err == nil {
					err = os.Symlink(path+".original", path)
				}
			case "canceled":
				cancel()
			}
			if err != nil {
				t.Fatal(err)
			}
			if fault == "proposal" || fault == "duplicate-key" || fault == "unknown-key" || fault == "trailing-data" {
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			loaded, err := e.loadGuestIdentityNameServiceIntent(ctx, owner)
			if fault == "none" {
				if err != nil || loaded.OwnerID != owner || loaded.Original != string(original) || loaded.Proposal != proposal {
					t.Fatal("committed selection did not reload", err)
				}
				return
			}
			if err == nil || loaded != (guestIdentityNameServiceIntent{}) {
				t.Fatal("invalid record supplied restart authority", err)
			}
			if fault == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatal("cancellation lost", err)
			}
		})
	}
}
