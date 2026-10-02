package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProvenancePreparationRequiresExactReviewedPin(t *testing.T) {
	data := []byte(`{"schema":1,"threshold":2,"keys":["` + strings.Repeat("ab", 32) + `","` + strings.Repeat("cd", 32) + `"],"builderId":"fixture","buildType":"fixture","externalParameters":{},"sourceUri":"fixture","sourceCommit":"` + strings.Repeat("ef", 20) + `"}`)
	path := filepath.Join(t.TempDir(), "policy.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	pin := hex.EncodeToString(sum[:])
	actual, err := readPinnedProvenancePolicy(path, pin)
	if err != nil || !bytes.Equal(actual, data) {
		t.Fatal("pinned policy refused", err)
	}
	for _, wrong := range []string{"", strings.Repeat("0", 64), strings.ToUpper(pin)} {
		if data, err := readPinnedProvenancePolicy(path, wrong); err == nil || data != nil {
			t.Fatal("wrong policy pin accepted")
		}
	}
	link := path + ".link"
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if data, err := readPinnedProvenancePolicy(link, pin); err == nil || data != nil {
		t.Fatal("symlink input admitted")
	}
}
