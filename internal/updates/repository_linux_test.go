//go:build linux

package updates

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	release := releaseMetadataFixture()
	customBytes, err := json.Marshal(release)
	if err != nil {
		t.Fatal(err)
	}
	custom := json.RawMessage(customBytes)
	packageName := "releases/0.1.0/homenode.deb"
	targets.Signed.Targets[packageName] = &metadata.TargetFiles{Length: 15, Hashes: metadata.Hashes{"sha256": hash[:]}, Custom: &custom}
	evidenceBytes := map[string][]byte{release.SBOMTarget: []byte(`{"components":[]}`), release.ProvenanceTarget: []byte(`{"buildType":"fixture"}`)}
	for name, data := range evidenceBytes {
		sum := sha256.Sum256(data)
		targets.Signed.Targets[name] = &metadata.TargetFiles{Length: int64(len(data)), Hashes: metadata.Hashes{"sha256": sum[:]}}
	}
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
	files["/targets/releases/0.1.0/"+hex.EncodeToString(hash[:])+".homenode.deb"] = []byte("release fixture")
	for name, data := range evidenceBytes {
		descriptor := targets.Signed.Targets[name]
		basename := filepath.Base(name)
		files["/targets/"+filepath.Dir(name)+"/"+hex.EncodeToString(descriptor.Hashes["sha256"])+"."+basename] = data
	}
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
	stagingDirectory := t.TempDir()
	if err = os.Chmod(stagingDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	staging, err := os.OpenRoot(stagingDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer staging.Close()
	targetFetcher, err := newMetadataFetcher(context.Background(), server.URL+"/targets")
	if err != nil {
		t.Fatal(err)
	}
	targetFetcher.client.Transport = server.Client().Transport
	defer targetFetcher.client.CloseIdleConnections()
	acquired, err := first.acquirePackageWithFetcher(packageName, targetFetcher, staging, ReleasePolicy{MinimumSequence: 5, MinimumCatalogVersion: 3, CurrentStateSchema: 4})
	if err != nil {
		first.Close()
		t.Fatal("signed release acquisition failed", err)
	}
	contents, readErr := io.ReadAll(acquired.Package)
	closeErr := acquired.Close()
	if readErr != nil || closeErr != nil || string(contents) != "release fixture" || acquired.Metadata != release || string(acquired.SBOM) != string(evidenceBytes[release.SBOMTarget]) || string(acquired.Provenance) != string(evidenceBytes[release.ProvenanceTarget]) {
		first.Close()
		t.Fatal("acquired evidence/package mismatch", readErr, closeErr)
	}
	mutex.Lock()
	beforeDenied := len(requests)
	mutex.Unlock()
	deniedRelease, deniedErr := first.acquirePackageWithFetcher(packageName, targetFetcher, staging, ReleasePolicy{MinimumSequence: 6, MinimumCatalogVersion: 3, CurrentStateSchema: 4})
	if deniedRelease != nil {
		deniedRelease.Close()
	}
	if deniedErr == nil {
		first.Close()
		t.Fatal("release below security floor acquired")
	}
	mutex.Lock()
	afterDenied := len(requests)
	mutex.Unlock()
	if beforeDenied != afterDenied {
		first.Close()
		t.Fatal("denied release caused a target request")
	}
	sbomDescriptor := targets.Signed.Targets[release.SBOMTarget]
	sbomURL := "/targets/" + filepath.Dir(release.SBOMTarget) + "/" + hex.EncodeToString(sbomDescriptor.Hashes["sha256"]) + "." + filepath.Base(release.SBOMTarget)
	badSBOM := append([]byte(nil), evidenceBytes[release.SBOMTarget]...)
	badSBOM[0] ^= 1
	mutex.Lock()
	files[sbomURL] = badSBOM
	beforeBadEvidence := len(requests)
	mutex.Unlock()
	deniedEvidence, evidenceErr := first.acquirePackageWithFetcher(packageName, targetFetcher, staging, ReleasePolicy{MinimumSequence: 5, MinimumCatalogVersion: 3, CurrentStateSchema: 4})
	if deniedEvidence != nil {
		deniedEvidence.Close()
	}
	if evidenceErr == nil {
		first.Close()
		t.Fatal("corrupt signed evidence acquired")
	}
	mutex.Lock()
	requestedPackage := false
	for _, request := range requests[beforeBadEvidence:] {
		if strings.HasSuffix(request, ".homenode.deb") {
			requestedPackage = true
		}
	}
	files[sbomURL] = evidenceBytes[release.SBOMTarget]
	mutex.Unlock()
	if requestedPackage {
		first.Close()
		t.Fatal("package requested after corrupt evidence")
	}
	if err = first.Close(); err != nil {
		t.Fatal(err)
	}
	joinedDirectory := t.TempDir()
	if err = os.Chmod(joinedDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	joinedStaging, err := os.OpenRoot(joinedDirectory)
	if err != nil {
		t.Fatal(err)
	}
	defer joinedStaging.Close()
	joinedMetadata, err := newMetadataFetcher(context.Background(), server.URL+"/metadata")
	if err != nil {
		t.Fatal(err)
	}
	joinedMetadata.client.Transport = server.Client().Transport
	defer joinedMetadata.client.CloseIdleConnections()
	joinedRelease, err := acquireReleaseWithFetchers(context.Background(), provisioned, joinedStaging, packageName, ReleasePolicy{MinimumSequence: 5, MinimumCatalogVersion: 3, CurrentStateSchema: 4}, joinedMetadata, targetFetcher)
	if err != nil {
		t.Fatal("joined refresh and acquisition failed", err)
	}
	joinedBytes, joinedReadErr := io.ReadAll(joinedRelease.Package)
	joinedCloseErr := joinedRelease.Close()
	if joinedReadErr != nil || joinedCloseErr != nil || string(joinedBytes) != "release fixture" || joinedRelease.Metadata != release {
		t.Fatal("joined acquisition mismatch", joinedReadErr, joinedCloseErr)
	}
	// Acquiring again must fail on retained publication rather than keep the
	// cache locked or silently adopt the prior package as a new operation.
	joinedRelease, err = acquireReleaseWithFetchers(context.Background(), provisioned, joinedStaging, packageName, ReleasePolicy{MinimumSequence: 5, MinimumCatalogVersion: 3, CurrentStateSchema: 4}, joinedMetadata, targetFetcher)
	if err == nil || joinedRelease != nil {
		t.Fatal("retained package silently adopted")
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
	// Replacement roots must satisfy both the old and new root roles.
	replacement, err := metadata.Root().FromBytes(rotated)
	if err != nil {
		t.Fatal(err)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := metadata.KeyFromPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	replacement.Signed.Version = 3
	replacement.Signed.Roles[metadata.ROOT].KeyIDs = nil
	if err = replacement.Signed.AddKey(key, metadata.ROOT); err != nil {
		t.Fatal(err)
	}
	replacementSigner, err := signature.LoadSigner(private, crypto.Hash(0))
	if err != nil {
		t.Fatal(err)
	}
	unauthorized := signedFixture(t, replacement, replacementSigner)
	mutex.Lock()
	files["/metadata/3.root.json"] = unauthorized
	mutex.Unlock()
	deniedRotation, rotationErr := open()
	if deniedRotation != nil {
		deniedRotation.Close()
	}
	if rotationErr == nil {
		t.Fatal("replacement key authorized itself without old root")
	}
	current, err = os.ReadFile(rootName)
	if err != nil || string(current) != string(rotated) {
		t.Fatal("unapproved replacement root persisted", err)
	}
	replacement.ClearSignatures()
	if _, err = replacement.Sign(signers[metadata.ROOT]); err != nil {
		t.Fatal(err)
	}
	if _, err = replacement.Sign(replacementSigner); err != nil {
		t.Fatal(err)
	}
	authorized, err := replacement.ToBytes(false)
	if err != nil {
		t.Fatal(err)
	}
	mutex.Lock()
	files["/metadata/3.root.json"] = authorized
	mutex.Unlock()
	rotatedSession, err := open()
	if err != nil {
		t.Fatal("authorized root key replacement rejected", err)
	}
	if err = rotatedSession.Close(); err != nil {
		t.Fatal(err)
	}
	current, err = os.ReadFile(rootName)
	if err != nil || string(current) != string(authorized) {
		t.Fatal("replacement root not retained", err)
	}
	afterReplacement, err := open()
	if err != nil {
		t.Fatal("replacement root not usable after restart", err)
	}
	if err = afterReplacement.Close(); err != nil {
		t.Fatal(err)
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
