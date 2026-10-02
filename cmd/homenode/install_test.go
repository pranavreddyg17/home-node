package main

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/catalog"
	"github.com/sigstore/sigstore/pkg/signature"
	"github.com/theupdateframework/go-tuf/v2/metadata"
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
		{"--update-root", "missing"}, {"--update-root-sha256", "00"},
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

func TestPreparationAcceptsIndependentlyPinnedThresholdUpdateRoot(t *testing.T) {
	args, now := preparationFixture(t)
	var updateRootSigners []signature.Signer
	root := metadata.Root(now.Add(time.Hour))
	root.Signed.Roles[metadata.ROOT].Threshold = 2
	for _, role := range []string{metadata.ROOT, metadata.TIMESTAMP, metadata.SNAPSHOT, metadata.TARGETS} {
		count := 1
		if role == metadata.ROOT {
			count = 2
		}
		for i := 0; i < count; i++ {
			public, private, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			key, err := metadata.KeyFromPublicKey(public)
			if err != nil {
				t.Fatal(err)
			}
			if err = root.Signed.AddKey(key, role); err != nil {
				t.Fatal(err)
			}
			if role == metadata.ROOT {
				signer, err := signature.LoadSigner(private, crypto.Hash(0))
				if err != nil {
					t.Fatal(err)
				}
				// Sign only after every key has been added to the root below.
				updateRootSigners = append(updateRootSigners, signer)
			}
		}
	}
	for _, signer := range updateRootSigners {
		if _, err := root.Sign(signer); err != nil {
			t.Fatal(err)
		}
	}
	data, err := root.ToBytes(false)
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(t.TempDir(), "root.json")
	if err = os.WriteFile(name, data, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	pin := hex.EncodeToString(sum[:])
	trustedArgs := append(args, "--update-root", name, "--update-root-sha256", pin)
	p, err := parsePreparation(trustedArgs, now)
	if err != nil || p.configuration.UpdateBootstrap == nil || p.configuration.UpdateBootstrap.SHA256 != pin || string(p.configuration.UpdateBootstrap.Data) != string(data) {
		t.Fatal("pinned bootstrap omitted", err)
	}
	p, err = parsePreparation(append(trustedArgs, "--update-metadata-url", "https://updates.example/metadata/", "--update-targets-url", "https://updates.example/targets/", "--update-sequence-floor", "5"), now)
	if err != nil || p.configuration.UpdateRepository == nil || p.configuration.UpdateRepository.MinimumSequence != 5 || p.configuration.UpdateRepository.MinimumCatalogVersion != p.configuration.MinimumCatalogVersion {
		t.Fatal("repository policy omitted", err)
	}
	provenance := []byte(`{"schema":1,"threshold":2,"keys":["` + strings.Repeat("ab", 32) + `","` + strings.Repeat("cd", 32) + `"],"builderId":"fixture","buildType":"fixture","externalParameters":{},"sourceUri":"fixture","sourceCommit":"` + strings.Repeat("ef", 20) + `"}`)
	provenancePath := filepath.Join(t.TempDir(), "provenance-policy.json")
	if err := os.WriteFile(provenancePath, provenance, 0600); err != nil {
		t.Fatal(err)
	}
	provenanceSum := sha256.Sum256(provenance)
	provenancePin := hex.EncodeToString(provenanceSum[:])
	repositoryArgs := append(append([]string{}, trustedArgs...), "--update-metadata-url", "https://updates.example/metadata/", "--update-targets-url", "https://updates.example/targets/", "--update-sequence-floor", "5")
	completeArgs := append(append([]string{}, repositoryArgs...), "--update-provenance", provenancePath, "--update-provenance-sha256", provenancePin)
	p, err = parsePreparation(completeArgs, now)
	if err != nil || string(p.configuration.UpdateProvenance) != string(provenance) {
		t.Fatal("reviewed provenance omitted from full preparation", err)
	}
	for _, incomplete := range [][]string{
		append(append([]string{}, repositoryArgs...), "--update-provenance", provenancePath),
		append(append([]string{}, repositoryArgs...), "--update-provenance-sha256", provenancePin),
		append(append([]string{}, trustedArgs...), "--update-provenance", provenancePath, "--update-provenance-sha256", provenancePin),
		append(append([]string{}, args...), "--update-provenance", provenancePath, "--update-provenance-sha256", provenancePin),
		append(append([]string{}, repositoryArgs...), "--update-provenance", provenancePath, "--update-provenance-sha256", strings.Repeat("0", 64)),
	} {
		if _, err := parsePreparation(incomplete, now); err == nil {
			t.Fatal("incomplete or unpinned provenance setup admitted")
		}
	}
	for _, extra := range [][]string{
		{"--update-metadata-url", "https://updates.example/metadata/"},
		{"--update-targets-url", "https://updates.example/targets/"},
		{"--update-sequence-floor", "5"},
		{"--update-metadata-url", "https://updates.example/metadata/", "--update-targets-url", "https://other.example/targets/", "--update-sequence-floor", "5"},
	} {
		if _, err := parsePreparation(append(trustedArgs, extra...), now); err == nil {
			t.Fatal("partial or cross-host repository accepted", extra)
		}
	}

}
