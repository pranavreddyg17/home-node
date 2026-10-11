package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestGatewayTLSRetirementJournalRetainsPrivateBoundedSingleLinkFile(t *testing.T) {
	host, jr := roots(t)
	e := openEngine(t, host, jr)
	data := []byte(`{"version":1}`)
	if err := e.saveJournalBytes("gateway-tls-retirement", data); err != nil {
		t.Fatal(err)
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	e = openEngine(t, host, jr)
	defer e.Close()
	got, err := e.readGatewayTLSRetirementIntent()
	if err != nil || string(got) != string(data) {
		t.Fatal(string(got), err)
	}
	name := filepath.Join(jr, "gateway-tls-retirement.json")
	if err := os.Chmod(name, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := e.readGatewayTLSRetirementIntent(); err == nil {
		t.Fatal("public intent admitted")
	}
	if err := os.Chmod(name, 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(jr, "foreign-link")
	if err := os.Link(name, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := e.readGatewayTLSRetirementIntent(); err == nil {
		t.Fatal("hardlinked intent admitted")
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, make([]byte, gatewayTLSRetirementJournalLimit+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.readGatewayTLSRetirementIntent(); err == nil {
		t.Fatal("oversized intent admitted")
	}
	if err := os.Remove(name); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(alias, name); err != nil {
		t.Fatal(err)
	}
	if _, err := e.readGatewayTLSRetirementIntent(); err == nil {
		t.Fatal("symlink intent admitted")
	}
}
func TestGatewayTLSRetirementPreparationRefusesMissingAuthorityAndCancellation(t *testing.T) {
	host, jr := roots(t)
	e := openEngine(t, host, jr)
	defer e.Close()
	if err := e.prepareGatewayTLSRetirementIntentLocked(context.Background(), gatewayTLSRetirementIntent{}, nil); err == nil {
		t.Fatal("missing guard admitted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := e.prepareGatewayTLSRetirementIntentLocked(ctx, gatewayTLSRetirementIntent{}, func() error { called = true; return nil })
	if !errors.Is(err, context.Canceled) || called {
		t.Fatal(err, called)
	}
	if _, err := os.Lstat(filepath.Join(jr, "gateway-tls-retirement.json")); !os.IsNotExist(err) {
		t.Fatal("unauthorized preparation persisted intent", err)
	}
}
