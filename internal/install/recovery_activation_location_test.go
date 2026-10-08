package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRecoveryJournalLocationRejectsDifferentOrReplacedDirectory(t *testing.T) {
	host, unrelated := roots(t)
	e := openEngine(t, host, unrelated)
	if err := e.requireRecoveryJournalLocation(context.Background()); !errors.Is(err, ErrConflict) {
		t.Fatal("unrelated journal supplied fixed activation authority", err)
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(host, "var/lib/homenode-install")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	e = openEngine(t, host, path)
	defer e.Close()
	if err := e.requireRecoveryJournalLocation(context.Background()); err != nil {
		t.Fatal("fixed owned journal refused", err)
	}
	if err := os.Rename(path, path+".retained"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := e.requireRecoveryJournalLocation(context.Background()); !errors.Is(err, ErrConflict) {
		t.Fatal("replaced directory retained activation authority", err)
	}
}
