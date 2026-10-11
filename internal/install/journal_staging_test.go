package install

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJournalPublicationRefusesHardlinkedOrOversizedTornStaging(t *testing.T) {
	for _, kind := range []string{"hardlink", "oversized", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			host, jr := roots(t)
			e := openEngine(t, host, jr)
			defer e.Close()
			committed := []byte("existing committed intent")
			if err := e.saveJournalBytes("gateway-tls-retirement", committed); err != nil {
				t.Fatal(err)
			}
			staged := filepath.Join(jr, "gateway-tls-retirement.next")
			alias := filepath.Join(jr, "foreign-stage")
			contents := []byte("foreign staging data")
			if kind == "oversized" {
				contents = make([]byte, (64<<10)+1)
			}
			if kind == "symlink" {
				if err := os.WriteFile(alias, contents, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(alias, staged); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.WriteFile(staged, contents, 0600); err != nil {
					t.Fatal(err)
				}
				if kind == "hardlink" {
					if err := os.Link(staged, alias); err != nil {
						t.Fatal(err)
					}
				}
			}
			before, err := os.Lstat(staged)
			if err != nil {
				t.Fatal(err)
			}
			if err := e.saveJournalBytes("gateway-tls-retirement", []byte("replacement")); err == nil {
				t.Fatal("foreign staging entry admitted")
			}
			after, err := os.Lstat(staged)
			if err != nil || !os.SameFile(before, after) {
				t.Fatal("foreign staging entry removed", err)
			}
			data, err := e.readGatewayTLSRetirementIntent()
			if err != nil || string(data) != string(committed) {
				t.Fatal("committed intent changed", err)
			}
		})
	}
}
func TestJournalPublicationReplacesOwnedSingleLinkTornStaging(t *testing.T) {
	host, jr := roots(t)
	e := openEngine(t, host, jr)
	defer e.Close()
	if err := os.WriteFile(filepath.Join(jr, "gateway-tls-retirement.next"), []byte("interrupted own write"), 0600); err != nil {
		t.Fatal(err)
	}
	expected := []byte("new committed intent")
	if err := e.saveJournalBytes("gateway-tls-retirement", expected); err != nil {
		t.Fatal(err)
	}
	data, err := e.readGatewayTLSRetirementIntent()
	if err != nil || string(data) != string(expected) {
		t.Fatal(string(data), err)
	}
}
