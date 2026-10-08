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
	var saved guestIdentityNameServiceIntent
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := e.withGuestIdentityNameServiceIntent(context.Background(), saved, func(context.Context) error { called = true; return nil }); err != nil || !called {
		t.Fatal("durable intent scope refused", err)
	}
	err = e.withGuestIdentityNameServiceIntent(context.Background(), saved, func(context.Context) error {
		if err := os.Rename(path, path+".retained"); err != nil {
			return err
		}
		return os.WriteFile(path, data, 0600)
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatal("identical-byte replacement survived intent scope", err)
	}
}
