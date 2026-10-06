package install

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestRecoveryIntentPreservesForeignAndTornWrites(t *testing.T) {
	host, journal := roots(t)
	e := openEngine(t, host, journal)
	defer e.Close()
	ctx := context.Background()
	canonical := []byte(`{"version":1,"fixture":"selected recovery"}`)
	if err := e.commitRecoveryIntent(ctx, canonical); err != nil {
		t.Fatal(err)
	}
	before, err := e.journalRoot.Lstat("recovery.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.commitRecoveryIntent(ctx, canonical); err != nil {
		t.Fatal("exact retry", err)
	}
	after, err := e.journalRoot.Lstat("recovery.json")
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("retry replaced intent", err)
	}
	if err = e.commitRecoveryIntent(ctx, []byte(`{"version":1,"fixture":"foreign recovery!"}`)); !errors.Is(err, ErrConflict) {
		t.Fatal("foreign recovery adopted", err)
	}
	if err = e.journalRoot.WriteFile("recovery.json", []byte(`{"version":`), 0600); err != nil {
		t.Fatal(err)
	}
	if err = e.commitRecoveryIntent(ctx, canonical); !errors.Is(err, ErrConflict) {
		t.Fatal("torn intent replaced", err)
	}
	got, err := e.journalRoot.ReadFile("recovery.json")
	if err != nil || string(got) != `{"version":` {
		t.Fatal("torn bytes lost", string(got), err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err = e.commitRecoveryIntent(cancelled, canonical); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel ignored", err)
	}
}
