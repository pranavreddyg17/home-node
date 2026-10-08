package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRecoveryRecordRefusesHardLinkAliasWithoutMutation(t *testing.T) {
	for _, name := range []string{"recovery.json", "recovery-staged.json"} {
		t.Run(name, func(t *testing.T) {
			host, journal := roots(t)
			e := openEngine(t, host, journal)
			defer e.Close()
			data := []byte(`{"version":1,"fixture":"immutable"}`)
			if err := e.commitRecoveryRecord(context.Background(), name, data); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(journal, name)
			alias := filepath.Join(journal, name+".alias")
			if err := os.Link(path, alias); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := e.commitRecoveryRecord(context.Background(), name, data); !errors.Is(err, ErrConflict) {
				t.Fatal("aliased retry admitted", err)
			}
			if err := e.matchRecoveryRecord(context.Background(), name, data); !errors.Is(err, ErrConflict) {
				t.Fatal("aliased record matched", err)
			}
			for _, retained := range []string{path, alias} {
				current, err := os.Stat(retained)
				if err != nil || !os.SameFile(before, current) {
					t.Fatal("refusal replaced aliased intent", err)
				}
				contents, err := os.ReadFile(retained)
				if err != nil || string(contents) != string(data) {
					t.Fatal("refusal changed intent", err)
				}
			}
		})
	}
}

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
	receipt := []byte(`{"version":1,"intentSha256":"fixture","files":1}`)
	if err = e.commitRecoveryRecord(ctx, "recovery-staged.json", receipt); err != nil {
		t.Fatal("staging receipt", err)
	}
	if err = e.commitRecoveryRecord(ctx, "recovery-staged.json", receipt); err != nil {
		t.Fatal("staging receipt retry", err)
	}
	if err = e.commitRecoveryRecord(ctx, "recovery-staged.json", []byte(`{"version":1,"files":2}`)); !errors.Is(err, ErrConflict) {
		t.Fatal("foreign staging receipt adopted", err)
	}
	if err = e.matchRecoveryRecord(ctx, "recovery-staged.json", receipt); err != nil {
		t.Fatal("matching receipt refused", err)
	}
	if err = e.matchRecoveryRecord(ctx, "recovery-staged.json", []byte(`{"version":2}`)); !errors.Is(err, ErrConflict) {
		t.Fatal("foreign receipt matched", err)
	}
	if err = e.journalRoot.Remove("recovery-staged.json"); err != nil {
		t.Fatal(err)
	}
	if err = e.matchRecoveryRecord(ctx, "recovery-staged.json", receipt); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("missing record matched", err)
	}
	if _, err = e.journalRoot.Lstat("recovery-staged.json"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("matching created record", err)
	}
	if err = e.commitRecoveryRecord(ctx, "../escape", receipt); !errors.Is(err, ErrPlan) {
		t.Fatal("foreign journal path admitted", err)
	}
}
