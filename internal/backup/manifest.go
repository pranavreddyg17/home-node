package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"time"
	"unicode/utf8"
)

const MaxManifestBytes = 64 << 10

var ErrManifest = errors.New("backup manifest is invalid or incompatible")
var releasePattern = regexp.MustCompile(`^[0-9][a-zA-Z0-9.+~-]{0,63}$`)

// Manifest contains compatibility and integrity metadata, never credentials,
// absolute host paths, network identity or runtime start authority.
type Manifest struct {
	Version          int          `json:"version"`
	CreatedAt        time.Time    `json:"createdAt"`
	Release          string       `json:"release"`
	Platform         string       `json:"platform"`
	ManagementSchema int          `json:"managementSchema"`
	CatalogVersion   int64        `json:"catalogVersion"`
	Files            []BackupFile `json:"files"`
}
type BackupFile struct {
	Workload    string `json:"workload"`
	Name        string `json:"name"`
	Bytes       int64  `json:"bytes"`
	SHA256      string `json:"sha256"`
	ImageSHA256 string `json:"imageSha256,omitempty"`
	DataSchema  int    `json:"dataSchema"`
	Protocol    int    `json:"protocol"`
}
type RestorePolicy struct {
	MinimumCatalogVersion int64
	// ApprovedImages comes from the replacement host's verified catalog, never
	// from the restored manifest itself.
	ApprovedImages map[string]string
}

func DecodeManifest(data []byte) (Manifest, error) {
	var m Manifest
	if len(data) == 0 || len(data) > MaxManifestBytes || !utf8.Valid(data) {
		return m, ErrManifest
	}
	// Avoid ambiguous duplicate security fields rather than letting JSON's
	// last-value-wins behavior decide restore policy.
	if err := uniqueJSON(json.NewDecoder(bytes.NewReader(data)), 0); err != nil {
		return m, ErrManifest
	}
	if err := requireManifestFields(data); err != nil {
		return m, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&m); err != nil {
		return m, ErrManifest
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return m, ErrManifest
	}
	return m, nil
}

func uniqueJSON(decoder *json.Decoder, depth int) error {
	if depth > 16 {
		return ErrManifest
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return ErrManifest
			}
			seen[name] = true
			if err := uniqueJSON(decoder, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := uniqueJSON(decoder, depth+1); err != nil {
				return err
			}
		}
	default:
		return ErrManifest
	}
	_, err = decoder.Token()
	return err
}
func (m Manifest) Validate(policy RestorePolicy, now time.Time) error {
	if m.Version != 1 || !releasePattern.MatchString(m.Release) || m.Platform != "ubuntu-24.04-amd64" || m.ManagementSchema != 4 || m.CatalogVersion < 1 || policy.MinimumCatalogVersion < 1 || m.CatalogVersion < policy.MinimumCatalogVersion || m.CreatedAt.IsZero() || m.CreatedAt.After(now.Add(5*time.Minute)) || len(m.Files) < 1 || len(m.Files) > 3 {
		return ErrManifest
	}
	seen := map[string]bool{}
	for _, f := range m.Files {
		if seen[f.Workload] || !repositoryPattern.MatchString(f.SHA256) || f.Bytes <= 0 {
			return ErrManifest
		}
		seen[f.Workload] = true
		switch f.Workload {
		case "management":
			if f.Name != "snapshot.db" || f.Bytes > 256<<20 || f.ImageSHA256 != "" || f.DataSchema != 4 || f.Protocol != 0 {
				return ErrManifest
			}
		case "files", "ai":
			if f.Name != f.Workload+".raw" || f.Bytes > 512<<30 || f.DataSchema != 1 || f.Protocol != 1 || !repositoryPattern.MatchString(f.ImageSHA256) || policy.ApprovedImages[f.Workload] != f.ImageSHA256 {
				return ErrManifest
			}
		default:
			return ErrManifest
		}
	}
	if !seen["management"] {
		return ErrManifest
	}
	return nil
}

// VerifyPayload reads only the fixed manifest file names beneath a staging root.
// A verified payload still needs SQLite identity sanitization, disconnected guest
// restore, filesystem ownership checks and fresh enrollment before activation.
func VerifyPayload(ctx context.Context, root *os.Root, manifest Manifest, policy RestorePolicy) error {
	if root == nil || manifest.Validate(policy, time.Now()) != nil {
		return ErrManifest
	}
	for _, entry := range manifest.Files {
		if err := verifyBackupFile(ctx, root, entry); err != nil {
			return err
		}
	}
	return nil
}
func verifyBackupFile(ctx context.Context, root *os.Root, entry BackupFile) error {
	info, err := root.Lstat(entry.Name)
	if err != nil || !info.Mode().IsRegular() {
		return ErrManifest
	}
	file, err := root.Open(entry.Name)
	if err != nil {
		return ErrManifest
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != entry.Bytes {
		return ErrManifest
	}
	hash := sha256.New()
	remaining := entry.Bytes
	buffer := make([]byte, 1<<20)
	for remaining > 0 {
		if err = ctx.Err(); err != nil {
			return err
		}
		size := min(int64(len(buffer)), remaining)
		n, err := io.ReadFull(file, buffer[:size])
		if err != nil {
			return ErrManifest
		}
		_, _ = hash.Write(buffer[:n])
		remaining -= int64(n)
	}
	var extra [1]byte
	if n, err := file.Read(extra[:]); n != 0 || err != io.EOF {
		return ErrManifest
	}
	if hex.EncodeToString(hash.Sum(nil)) != entry.SHA256 {
		return ErrManifest
	}
	return nil
}

func exactManifestObject(raw []byte, required []string, optional string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, ErrManifest
	}
	allowed := map[string]bool{}
	for _, key := range required {
		allowed[key] = true
		if value, ok := fields[key]; !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, ErrManifest
		}
	}
	if optional != "" {
		allowed[optional] = true
	}
	for key, value := range fields {
		if !allowed[key] || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, ErrManifest
		}
	}
	return fields, nil
}
func requireManifestFields(raw []byte) error {
	fields, err := exactManifestObject(raw, []string{"version", "createdAt", "release", "platform", "managementSchema", "catalogVersion", "files"}, "")
	if err != nil {
		return err
	}
	var files []json.RawMessage
	if json.Unmarshal(fields["files"], &files) != nil {
		return ErrManifest
	}
	for _, file := range files {
		if _, err := exactManifestObject(file, []string{"workload", "name", "bytes", "sha256", "dataSchema", "protocol"}, "imageSha256"); err != nil {
			return err
		}
	}
	return nil
}
