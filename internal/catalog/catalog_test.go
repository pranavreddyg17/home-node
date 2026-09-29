package catalog

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
)

func TestPublisherExpiryAndRollback(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	m := Manifest{Schema: 1, Version: 3, Expires: now.Add(time.Hour), Images: []Image{{ID: "files", SHA256: hex.EncodeToString(make([]byte, 32)), Bytes: 1, MemoryMiB: 512, VCPUs: 1, DataBytes: GiB, Protocol: 1, License: "inventory.json", Version: "1"}}}
	sign := func(m Manifest) []byte {
		payload, _ := json.Marshal(m)
		data, _ := json.Marshal(Envelope{KeyID: KeyID(pub), Payload: payload, Signature: ed25519.Sign(priv, payload)})
		return data
	}
	keys := map[string]ed25519.PublicKey{KeyID(pub): pub}
	if _, err = Verify(sign(m), keys, 3, now); err != nil {
		t.Fatal(err)
	}
	if _, err = Verify(sign(m), keys, 4, now); err == nil {
		t.Fatal("rollback accepted")
	}
	if _, err = Verify(sign(m), keys, 3, now.Add(2*time.Hour)); err == nil {
		t.Fatal("expired catalog accepted")
	}
	if _, err = Verify(sign(m), map[string]ed25519.PublicKey{}, 1, now); err == nil {
		t.Fatal("unknown publisher accepted")
	}
	m.Images[0].VCPUs = 100
	if _, err = Verify(sign(m), keys, 1, now); err == nil {
		t.Fatal("oversized resource grant accepted")
	}
}
func TestImageDigestAndSymlink(t *testing.T) {
	dir := t.TempDir()
	data := []byte("immutable raw image")
	hash := sha256.Sum256(data)
	image := Image{SHA256: hex.EncodeToString(hash[:]), Bytes: int64(len(data))}
	name := filepath.Join(dir, image.SHA256+".raw")
	if err := os.WriteFile(name, data, 0440); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyImage(dir, image); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(name, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte("modified"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyImage(dir, image); err == nil {
		t.Fatal("modified image accepted")
	}
	if err := os.Remove(name); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "other")
	os.WriteFile(other, data, 0600)
	os.Symlink(other, name)
	if _, err := VerifyImage(dir, image); err == nil {
		t.Fatal("symlink image accepted")
	}
}
