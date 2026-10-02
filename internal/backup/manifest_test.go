package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func manifestFixture() (Manifest, RestorePolicy, []byte) {
	data := []byte("consistent snapshot test fixture")
	hash := sha256.Sum256(data)
	return Manifest{Version: 1, CreatedAt: time.Now(), Release: "0.1.0~dev", Platform: "ubuntu-24.04-amd64", ManagementSchema: 4, CatalogVersion: 3, Files: []BackupFile{{Workload: "management", Name: "snapshot.db", Bytes: int64(len(data)), SHA256: hex.EncodeToString(hash[:]), DataSchema: 4}}}, RestorePolicy{MinimumCatalogVersion: 2}, data
}
func TestBackupPayloadIntegrityAndConfinement(t *testing.T) {
	manifest, policy, data := manifestFixture()
	path := t.TempDir()
	if err := os.WriteFile(filepath.Join(path, "snapshot.db"), data, 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err = VerifyPayload(context.Background(), root, manifest, policy); err != nil {
		t.Fatal(err)
	}
	data[0] = 'X'
	if err = os.WriteFile(filepath.Join(path, "snapshot.db"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err = VerifyPayload(context.Background(), root, manifest, policy); err == nil {
		t.Fatal("corrupt backup passed")
	}
	if err = os.Remove(filepath.Join(path, "snapshot.db")); err != nil {
		t.Fatal(err)
	}
	if err = VerifyPayload(context.Background(), root, manifest, policy); err == nil {
		t.Fatal("missing backup file passed")
	}
	if err = os.Symlink(filepath.Join(t.TempDir(), "outside"), filepath.Join(path, "snapshot.db")); err != nil {
		t.Fatal(err)
	}
	if err = VerifyPayload(context.Background(), root, manifest, policy); err == nil {
		t.Fatal("symlink passed")
	}
	manifest.Files[0].Name = "../snapshot.db"
	if err = manifest.Validate(policy, time.Now()); err == nil {
		t.Fatal("host path passed")
	}
}
func TestManifestCompatibility(t *testing.T) {
	original, policy, _ := manifestFixture()
	for _, mutate := range []func(*Manifest){
		func(m *Manifest) { m.Files = nil },
		func(m *Manifest) { m.Version = 2 },
		func(m *Manifest) { m.ManagementSchema = 5 },
		func(m *Manifest) { m.Platform = "other-host" },
		func(m *Manifest) { m.CatalogVersion = 1 },
		func(m *Manifest) { m.CreatedAt = time.Now().Add(time.Hour) },
		func(m *Manifest) { m.Files[0].Bytes = 257 << 20 },
		func(m *Manifest) { m.Files = append(m.Files, m.Files[0]) },
	} {
		m := original
		m.Files = append([]BackupFile(nil), original.Files...)
		mutate(&m)
		if err := m.Validate(policy, time.Now()); err == nil {
			t.Fatal("incompatible manifest accepted", m)
		}
	}
	image := strings.Repeat("a", 64)
	policy.ApprovedImages = map[string]string{"ai": image}
	m := original
	m.Files = append(m.Files, BackupFile{Workload: "ai", Name: "ai.raw", Bytes: 1 << 30, SHA256: image, ImageSHA256: image, DataSchema: 1, Protocol: 1})
	if err := m.Validate(policy, time.Now()); err != nil {
		t.Fatal(err)
	}
	m.Files[1].ImageSHA256 = strings.Repeat("b", 64)
	if err := m.Validate(policy, time.Now()); err == nil {
		t.Fatal("unapproved image reference accepted")
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = DecodeManifest(data); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range [][]byte{append(data, []byte("{}")...), []byte(`{"version":1,"version":2}`), []byte(`{"version":1,"shell":"run"}`), make([]byte, MaxManifestBytes+1)} {
		if _, err = DecodeManifest(invalid); err == nil {
			t.Fatal("invalid envelope accepted")
		}
	}
}
func FuzzBackupManifest(f *testing.F) {
	manifest, _, _ := manifestFixture()
	data, _ := json.Marshal(manifest)
	f.Add(data)
	f.Fuzz(func(t *testing.T, data []byte) {
		m, err := DecodeManifest(data)
		if err == nil {
			_ = m.Validate(RestorePolicy{MinimumCatalogVersion: 1}, time.Now())
		}
	})
}

func TestManifestDecoderRequiresExactNonNullFields(t *testing.T) {
	manifest, _, _ := manifestFixture()
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeManifest(raw); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{
		strings.Replace(string(raw), `"version":1`, `"Version":1`, 1),
		strings.Replace(string(raw), `"protocol":0`, `"Protocol":0`, 1),
		strings.Replace(string(raw), `"protocol":0`, `"protocol":null`, 1),
		strings.Replace(string(raw), `"catalogVersion":3,`, "", 1),
		strings.Replace(string(raw), `"workload":"management"`, `"workload":null`, 1),
		strings.Replace(string(raw), `"release":"0.1.0~dev"`, `"release":"`+string([]byte{255})+`"`, 1),
	} {
		if _, err := DecodeManifest([]byte(invalid)); err == nil {
			t.Fatal("ambiguous manifest admitted")
		}
	}
}
