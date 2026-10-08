package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGuestIdentityIntentBindsOriginalAndRefusesConflictingRetry(t *testing.T) {
	host, journal := roots(t)
	e := openEngine(t, host, journal)
	defer e.Close()
	original := []byte("passwd: files systemd\ngroup: files\nshadow: files\n")
	proposal, err := planGuestIdentityNameServices(original)
	if err != nil {
		t.Fatal(err)
	}
	owner := strings.Repeat("a", 32)
	if err := e.commitGuestIdentityNameServices(context.Background(), owner, original, proposal); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(journal, "guest-identity-nss-intent.json")
	before, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.commitGuestIdentityNameServices(context.Background(), owner, original, proposal); err != nil {
		t.Fatal(err)
	}
	if err := e.commitGuestIdentityNameServices(context.Background(), strings.Repeat("b", 32), original, proposal); !errors.Is(err, ErrConflict) {
		t.Fatal("foreign owner adopted intent", err)
	}
	changed := proposal
	changed.Contents += "passwd: files\n"
	if err := e.commitGuestIdentityNameServices(context.Background(), owner, original, changed); !errors.Is(err, ErrConflict) {
		t.Fatal("unplanned bytes committed", err)
	}
	after, err := os.Lstat(path)
	retained, readErr := os.ReadFile(path)
	if err != nil || readErr != nil || !os.SameFile(before, after) || string(retained) != string(data) {
		t.Fatal("retry changed committed identity", err, readErr)
	}
}
