package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/catalog"
)

func preparationFixture(t *testing.T) ([]string, time.Time) {
	t.Helper()
	now := time.Now()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	manifest := catalog.Manifest{Schema: 1, Version: 4, Expires: now.Add(time.Hour)}
	for _, id := range []string{"files", "video", "ai"} {
		data := []byte(id + " release fixture")
		sum := sha256.Sum256(data)
		hash := hex.EncodeToString(sum[:])
		manifest.Images = append(manifest.Images, catalog.Image{ID: id, SHA256: hash, Bytes: int64(len(data)), MemoryMiB: 1024, VCPUs: 1, DataBytes: 16 * catalog.GiB, Protocol: 1, License: "fixture", Version: "1"})
		if err = os.WriteFile(filepath.Join(directory, hash+".raw"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	payload, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(catalog.Envelope{KeyID: catalog.KeyID(pub), Payload: payload, Signature: ed25519.Sign(key, payload)})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(directory, "catalog.json")
	if err = os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	return []string{"--publisher-key", hex.EncodeToString(pub), "--catalog", file, "--catalog-floor", "4", "--images", directory, "--bind", "100.100.1.2", "--origin", "https://home.example.ts.net:8787"}, now
}
func TestPreparationAuthenticatesReleaseBeforeEffects(t *testing.T) {
	args, now := preparationFixture(t)
	p, err := parsePreparation(args, now)
	if err != nil {
		t.Fatal(err)
	}
	if p.configuration.MinimumCatalogVersion != 4 || p.configuration.Network.Port != 8787 || p.configuration.Capacity.MemoryBytes != 0 {
		t.Fatal("invalid parsed preparation")
	}
	for _, extra := range [][]string{
		{"--publisher-key", "00"}, {"--catalog-floor", "5"}, {"--catalog-floor", "0"}, {"--origin", "http://home.example.ts.net:8787"},
		{"--cpus", "0"}, {"--generation", "0"}, {"--images", filepath.Join(t.TempDir(), "missing")}, {"--controller-uid", "800"}, {"unexpected"},
	} {
		if _, err = parsePreparation(append(append([]string{}, args...), extra...), now); err == nil {
			t.Fatal("invalid input admitted", extra)
		}
	}
	if _, err = parsePreparation(args, now.Add(2*time.Hour)); err == nil {
		t.Fatal("expired catalog admitted")
	}
}
func TestCatalogReaderRejectsSymlinkAndOversize(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "catalog")
	if err := os.WriteFile(name, make([]byte, catalog.MaxManifestBytes+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readReleaseCatalog(name); err == nil {
		t.Fatal("oversized input admitted")
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(name, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readReleaseCatalog(link); err == nil {
		t.Fatal("symlink input admitted")
	}
}
