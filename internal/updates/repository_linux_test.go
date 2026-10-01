//go:build linux

package updates

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sigstore/sigstore/pkg/signature"
	"github.com/theupdateframework/go-tuf/v2/metadata"
)

func signedFixture[T metadata.Roles](t *testing.T, role *metadata.Metadata[T], signer signature.Signer) []byte {
	t.Helper()
	role.ClearSignatures()
	if _, err := role.Sign(signer); err != nil {
		t.Fatal(err)
	}
	data, err := role.ToBytes(false)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestSignedRepositoryRefreshRotationAndRestartRollback(t *testing.T) {
	expires := time.Now().UTC().Add(time.Hour)
	root := metadata.Root(expires)
	signers := map[string]signature.Signer{}
	for _, role := range []string{metadata.ROOT, metadata.TIMESTAMP, metadata.SNAPSHOT, metadata.TARGETS} {
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
		signers[role], err = signature.LoadSigner(private, crypto.Hash(0))
		if err != nil {
			t.Fatal(err)
		}
	}
	bootstrap := signedFixture(t, root, signers[metadata.ROOT])
	root.Signed.Version = 2
	rotated := signedFixture(t, root, signers[metadata.ROOT])
	targets := metadata.Targets(expires)
	targets.Signed.Version = 2
	hash := sha256.Sum256([]byte("release fixture"))
	targets.Signed.Targets["release.json"] = &metadata.TargetFiles{Length: 15, Hashes: metadata.Hashes{"sha256": hash[:]}}
	targetsData := signedFixture(t, targets, signers[metadata.TARGETS])
	snapshot := metadata.Snapshot(expires)
	snapshot.Signed.Version = 2
	snapshot.Signed.Meta["targets.json"].Version = 2
	snapshotData := signedFixture(t, snapshot, signers[metadata.SNAPSHOT])
	timestamp := metadata.Timestamp(expires)
	timestamp.Signed.Version = 2
	timestamp.Signed.Meta["snapshot.json"].Version = 2
	timestampData := signedFixture(t, timestamp, signers[metadata.TIMESTAMP])
	var mutex sync.Mutex
	files := map[string][]byte{"/metadata/2.root.json": rotated, "/metadata/timestamp.json": timestampData, "/metadata/2.snapshot.json": snapshotData, "/metadata/2.targets.json": targetsData}
	var requests []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mutex.Lock()
		requests = append(requests, r.URL.Path)
		data, present := files[r.URL.Path]
		mutex.Unlock()
		if !present {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	}))
	defer server.Close()
	provisioned, directory := updateCacheFixture(t)
	rootName := filepath.Join(directory, "metadata", "root.json")
	if err := os.WriteFile(rootName, bootstrap, 0644); err != nil {
		t.Fatal(err)
	}
	open := func() (*verificationSession, error) {
		fetcher, err := newMetadataFetcher(context.Background(), server.URL+"/metadata")
		if err != nil {
			t.Fatal(err)
		}
		fetcher.client.Transport = server.Client().Transport
		return newVerificationSessionWithFetcher(context.Background(), provisioned, fetcher)
	}
	first, err := open()
	if err != nil {
		t.Fatal(err)
	}
	target, err := first.TargetInfo("release.json")
	if err != nil || target.Length != 15 || target.Path != "release.json" {
		first.Close()
		t.Fatal(target, err)
	}
	target.Length = 999
	again, err := first.TargetInfo("release.json")
	if err != nil || again.Length != 15 {
		first.Close()
		t.Fatal("trusted target mutated", again, err)
	}
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(rootName)
	if err != nil || string(current) != string(rotated) {
		t.Fatal("rotated root not retained", err)
	}
	mutex.Lock()
	requests = nil
	mutex.Unlock()
	second, err := open()
	if err != nil {
		t.Fatal("current repository rejected after restart", err)
	}
	if err = second.Close(); err != nil {
		t.Fatal(err)
	}
	mutex.Lock()
	fellBack := false
	for _, request := range requests {
		if request == "/metadata/2.root.json" {
			fellBack = true
		}
	}
	mutex.Unlock()
	if fellBack {
		t.Fatal("restart fell back to bootstrap root")
	}
	timestamp.Signed.Version = 1
	replay := signedFixture(t, timestamp, signers[metadata.TIMESTAMP])
	mutex.Lock()
	files["/metadata/timestamp.json"] = replay
	mutex.Unlock()
	rejected, err := open()
	if rejected != nil {
		rejected.Close()
	}
	if err == nil {
		t.Fatal("signed timestamp rollback accepted after restart")
	}
	retained, err := os.ReadFile(filepath.Join(directory, "metadata", "timestamp.json"))
	if err != nil || string(retained) != string(timestampData) {
		t.Fatal("rollback replaced trusted timestamp", err)
	}
	for _, attack := range []string{"expired", "wrong-role-signer"} {
		timestamp.Signed.Version = 3
		timestamp.Signed.Expires = expires
		signer := signers[metadata.TIMESTAMP]
		if attack == "expired" {
			timestamp.Signed.Expires = time.Now().UTC().Add(-time.Minute)
		} else {
			signer = signers[metadata.TARGETS]
		}
		malicious := signedFixture(t, timestamp, signer)
		mutex.Lock()
		files["/metadata/timestamp.json"] = malicious
		mutex.Unlock()
		denied, deniedErr := open()
		if denied != nil {
			denied.Close()
		}
		if deniedErr == nil {
			t.Fatal("invalid timestamp accepted", attack)
		}
		retained, err = os.ReadFile(filepath.Join(directory, "metadata", "timestamp.json"))
		if err != nil || string(retained) != string(timestampData) {
			t.Fatal("invalid timestamp replaced trusted state", attack, err)
		}
	}
	lock, err := lockMetadataCache(context.Background(), provisioned)
	if err != nil {
		t.Fatal("failed refresh retained cache ownership", err)
	}
	lock.Close()
}
