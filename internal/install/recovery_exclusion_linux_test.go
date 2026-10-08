//go:build linux

package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRootRecoveryExclusionRetainsMarkerAcrossConsumer(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_UPDATE_INIT_INTEGRATION") != "1" {
		t.Skip("explicit disposable Linux root fixture")
	}
	for _, fault := range []string{"none", "configuration-drift", "consumer-failed", "marker-replaced", "runtime-returned", "guard-configuration-drift", "identity-transaction"} {
		t.Run(fault, func(t *testing.T) {
			c, _, _, now := configurationFixture(t)
			c.Maintenance = &MaintenanceAccount{UID: 803, GID: 803}
			c.BackupRepositoryID = strings.Repeat("a", 64)
			c.BackupRelease, c.BackupDriveUUID = "0.1.0", "abcd-1234"
			preview, err := ConfigurationPlan(c, now)
			if err != nil {
				t.Fatal(err)
			}
			host, journal := roots(t)
			e := openEngine(t, host, journal)
			defer e.Close()
			if err := e.Apply(context.Background(), preview.Plan); err != nil {
				t.Fatal(err)
			}
			if err := e.blockRecoveryActivation(context.Background()); err != nil {
				t.Fatal(err)
			}
			original, err := e.journalRoot.Lstat("recovery-blocked")
			if err != nil {
				t.Fatal(err)
			}
			var identityIntent guestIdentityNameServiceIntent
			identityOriginal := []byte("passwd: files systemd\ngroup: files\nshadow: files\n")
			if fault == "identity-transaction" {
				proposal, err := planGuestIdentityNameServices(identityOriginal)
				if err != nil {
					t.Fatal(err)
				}
				identityIntent = guestIdentityNameServiceIntent{Version: 1, OwnerID: strings.Repeat("b", 32), Original: string(identityOriginal), Proposal: proposal}
				if err := os.WriteFile(filepath.Join(host, "etc/nsswitch.conf"), identityOriginal, 0644); err != nil {
					t.Fatal(err)
				}
				if err := e.commitGuestIdentityNameServices(context.Background(), identityIntent.OwnerID, identityOriginal, proposal); err != nil {
					t.Fatal(err)
				}
			}
			observations, consumers := 0, 0
			observe := func(context.Context) error {
				observations++
				if fault == "configuration-drift" && observations == 1 {
					return e.host.WriteFile("etc/homenode/runtime-policy.json", []byte("drift"), 0600)
				}
				if fault == "runtime-returned" && observations == 2 {
					return ErrConflict
				}
				return nil
			}
			consumerFailure := errors.New("publication interrupted")
			use := func(ctx context.Context) error {
				consumers++
				if fault == "consumer-failed" {
					return consumerFailure
				}
				if fault == "marker-replaced" {
					if err := e.journalRoot.Rename("recovery-blocked", "recovery-blocked.old"); err != nil {
						return err
					}
					return e.blockRecoveryActivation(ctx)
				}
				return nil
			}
			e.mu.Lock()
			err = e.withRecoveryExclusionGuardedLocked(context.Background(), observe, e.observeRecoveryDestinationVacancy, func(ctx context.Context, guard func(context.Context) error) error {
				if fault == "identity-transaction" {
					consumers++
					return e.applyGuestIdentityNameServicesLocked(ctx, identityIntent, guard)
				}
				if fault == "guard-configuration-drift" {
					if err := e.host.WriteFile("etc/homenode/runtime-policy.json", []byte("drift"), 0600); err != nil {
						return err
					}
					return guard(ctx)
				}
				return use(ctx)
			})
			e.mu.Unlock()
			switch fault {
			case "identity-transaction":
				if err != nil || observations < 3 || consumers != 1 {
					t.Fatal("identity mutation did not retain live exclusion checks", err, observations, consumers)
				}
				current, readErr := os.ReadFile(filepath.Join(host, "etc/nsswitch.conf"))
				originalBytes, originalErr := os.ReadFile(filepath.Join(host, "etc/.homenode-nsswitch.stage"))
				if readErr != nil || originalErr != nil || string(current) != identityIntent.Proposal.Contents || string(originalBytes) != string(identityOriginal) {
					t.Fatal("excluded identity transaction lost intended or original bytes", readErr, originalErr)
				}
			case "none":
				if err != nil || observations != 2 || consumers != 1 {
					t.Fatal("retained scope incomplete", observations, consumers, err)
				}
			case "consumer-failed":
				if !errors.Is(err, consumerFailure) || observations != 1 {
					t.Fatal("consumer failure lost or scope continued", err, observations)
				}
			default:
				if !errors.Is(err, ErrConflict) {
					t.Fatal("uncertain exclusion admitted", err)
				}
			}
			if fault == "configuration-drift" && consumers != 0 {
				t.Fatal("consumer ran after configuration drift")
			}
			retained, err := e.journalRoot.Lstat("recovery-blocked")
			if err != nil || retained.Size() != 0 || retained.Mode().Perm() != 0600 {
				t.Fatal("scope removed activation marker", err)
			}
			if fault != "marker-replaced" && !os.SameFile(original, retained) {
				t.Fatal("scope replaced activation marker")
			}
		})
	}
}
