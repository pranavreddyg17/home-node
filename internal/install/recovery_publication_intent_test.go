package install

import (
	"context"
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
