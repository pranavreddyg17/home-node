package install

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestRecoveryPublicationIntentPreservesExactAuthority(t *testing.T) {
	host, journal := roots(t)
	e := openEngine(t, host, journal)
	defer e.Close()
	intent := recoveryPublicationIntent{Version: 1, ConfigurationID: strings.Repeat("a", 32), ConfigurationDigest: strings.Repeat("b", 64), RecoveryIntentSHA256: strings.Repeat("c", 64), FileName: "management.db", ContentSHA256: strings.Repeat("d", 64), Identity: recoveryPublicationIdentity{Device: 1, Inode: 2, Bytes: 4096, UID: 801, GID: 801}}
	ctx := context.Background()
	if err := e.commitRecoveryPublicationIntent(ctx, intent); err != nil {
		t.Fatal(err)
	}
	name, _ := recoveryPublicationRecordName(intent.FileName)
	before, err := e.journalRoot.Lstat(name)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := e.journalRoot.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.commitRecoveryPublicationIntent(ctx, intent); err != nil {
		t.Fatal("exact retry", err)
	}
	changed := intent
	changed.Identity.Inode++
	if err := e.commitRecoveryPublicationIntent(ctx, changed); !errors.Is(err, ErrConflict) {
		t.Fatal("changed inode authority adopted", err)
	}
	changed = intent
	changed.RecoveryIntentSHA256 = strings.Repeat("e", 64)
	if err := e.commitRecoveryPublicationIntent(ctx, changed); !errors.Is(err, ErrConflict) {
		t.Fatal("changed recovery selection adopted", err)
	}
	current, err := e.journalRoot.Lstat(name)
	if err != nil || !os.SameFile(before, current) {
		t.Fatal("retry replaced authority", err)
	}
	retained, err := e.journalRoot.ReadFile(name)
	if err != nil || string(contents) != string(retained) {
		t.Fatal("refusal changed authority bytes", err)
	}
	for _, bad := range []string{"../management.db", "foreign.db", "/management.db"} {
		changed = intent
		changed.FileName = bad
		if err := e.commitRecoveryPublicationIntent(ctx, changed); !errors.Is(err, ErrPlan) {
			t.Fatal("foreign destination admitted", bad, err)
		}
	}
}

func TestRecoveryPublicationScopeRetainsBothRecords(t *testing.T) {
	for _, fault := range []string{"none", "replace-recovery", "replace-publication", "uncommitted-inode", "wrong-content"} {
		t.Run(fault, func(t *testing.T) {
			host, journal := roots(t)
			e := openEngine(t, host, journal)
			defer e.Close()
			recovery := recoveryIntent{Version: 1, ConfigurationID: strings.Repeat("a", 32), ConfigurationDigest: strings.Repeat("b", 64)}
			recovery.Recovery.ManagementSHA256 = strings.Repeat("d", 64)
			recovery.Recovery.ManagementBytes = 4096
			encodedRecovery, err := json.Marshal(recovery)
			if err != nil {
				t.Fatal(err)
			}
			if err := e.commitRecoveryIntent(context.Background(), encodedRecovery); err != nil {
				t.Fatal(err)
			}
			intent := recoveryPublicationIntent{Version: 1, ConfigurationID: recovery.ConfigurationID, ConfigurationDigest: recovery.ConfigurationDigest, RecoveryIntentSHA256: digest(encodedRecovery), FileName: "management.db", ContentSHA256: recovery.Recovery.ManagementSHA256, Identity: recoveryPublicationIdentity{Device: 1, Inode: 2, Bytes: 4096, UID: 801, GID: 801}}
			if err := e.commitRecoveryPublicationIntent(context.Background(), intent); err != nil {
				t.Fatal(err)
			}
			if fault == "uncommitted-inode" {
				intent.Identity.Inode++
			}
			if fault == "wrong-content" {
				intent.ContentSHA256 = strings.Repeat("e", 64)
			}
			called := false
			err = e.withRecoveryPublicationIntent(context.Background(), intent, recovery, func(context.Context) error {
				called = true
				name := "recovery.json"
				if fault == "replace-publication" {
					name, _ = recoveryPublicationRecordName(intent.FileName)
				}
				if fault != "replace-recovery" && fault != "replace-publication" {
					return nil
				}
				data, err := e.journalRoot.ReadFile(name)
				if err != nil {
					return err
				}
				if err := e.journalRoot.Rename(name, name+".old"); err != nil {
					return err
				}
				return e.journalRoot.WriteFile(name, data, 0600)
			})
			if fault == "none" {
				if err != nil || !called {
					t.Fatal("exact records refused", err)
				}
			} else if !errors.Is(err, ErrConflict) {
				t.Fatal("uncertain publication authority admitted", err)
			}
			if (fault == "uncommitted-inode" || fault == "wrong-content") && called {
				t.Fatal("consumer ran without committed matching authority")
			}
		})
	}
}
