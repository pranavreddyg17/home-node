// Package catalog verifies immutable workload descriptors against publisher
// keys installed by the trusted update path. This extra signature is checked
// by the supervisor independently of the API or update service.
package catalog

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

const MaxManifestBytes = 128 << 10
const GiB int64 = 1 << 30

var digestPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)
var ErrUntrusted = errors.New("catalog is untrusted, expired, incompatible or outside resource policy")

type Image struct {
	ID        string `json:"id"`
	SHA256    string `json:"sha256"`
	Bytes     int64  `json:"bytes"`
	MemoryMiB int    `json:"memoryMiB"`
	VCPUs     int    `json:"vcpus"`
	DataBytes int64  `json:"dataBytes"`
	Protocol  int    `json:"protocol"`
	License   string `json:"license"`
	Version   string `json:"version"`
}
type Manifest struct {
	Schema  int       `json:"schema"`
	Version int64     `json:"version"`
	Expires time.Time `json:"expires"`
	Images  []Image   `json:"images"`
}
type Envelope struct {
	KeyID     string `json:"keyId"`
	Payload   []byte `json:"payload"`
	Signature []byte `json:"signature"`
}

func KeyID(key ed25519.PublicKey) string {
	sum := sha256.Sum256(key)
	return hex.EncodeToString(sum[:])
}
func strict(data []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return ErrUntrusted
	}
	return nil
}
func Verify(data []byte, keys map[string]ed25519.PublicKey, minVersion int64, now time.Time) (Manifest, error) {
	var envelope Envelope
	var manifest Manifest
	if len(data) > MaxManifestBytes || strict(data, &envelope) != nil {
		return manifest, ErrUntrusted
	}
	key := keys[envelope.KeyID]
	if len(key) != ed25519.PublicKeySize || KeyID(key) != envelope.KeyID || !ed25519.Verify(key, envelope.Payload, envelope.Signature) {
		return manifest, ErrUntrusted
	}
	if strict(envelope.Payload, &manifest) != nil {
		return Manifest{}, ErrUntrusted
	}
	if manifest.Schema != 1 || manifest.Version < 1 || manifest.Version < minVersion || !manifest.Expires.After(now) || manifest.Expires.After(now.Add(90*24*time.Hour)) || len(manifest.Images) == 0 || len(manifest.Images) > 3 {
		return Manifest{}, ErrUntrusted
	}
	seen := map[string]bool{}
	for _, image := range manifest.Images {
		if seen[image.ID] || image.ID != "files" && image.ID != "video" && image.ID != "ai" || !digestPattern.MatchString(image.SHA256) || image.Protocol != 1 || image.Bytes < 1 || image.Bytes > 8*GiB || image.MemoryMiB < 256 || image.MemoryMiB > 12288 || image.VCPUs < 1 || image.VCPUs > 8 || image.DataBytes < GiB || image.DataBytes > 512*GiB || image.License == "" || image.Version == "" {
			return Manifest{}, ErrUntrusted
		}
		seen[image.ID] = true
	}
	return manifest, nil
}
func (m Manifest) Image(id string) (Image, error) {
	for _, image := range m.Images {
		if image.ID == id {
			return image, nil
		}
	}
	return Image{}, ErrUntrusted
}
func VerifyImage(directory string, image Image) (string, error) {
	if !digestPattern.MatchString(image.SHA256) {
		return "", ErrUntrusted
	}
	name := filepath.Join(directory, image.SHA256+".raw")
	file, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	link, err := os.Lstat(name)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || link.Mode()&os.ModeSymlink != 0 || info.Size() != image.Bytes || info.Mode().Perm()&0222 != 0 {
		return "", ErrUntrusted
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(file, image.Bytes+1))
	if err != nil {
		return "", err
	}
	if n != image.Bytes || hex.EncodeToString(hash.Sum(nil)) != image.SHA256 {
		return "", fmt.Errorf("image integrity: %w", ErrUntrusted)
	}
	return name, nil
}
