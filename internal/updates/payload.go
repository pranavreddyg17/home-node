package updates

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"path"
	"strings"
)

var ErrPackagePayload = errors.New("package payload is outside HomeNode release policy")

var packageExecutables = map[string]bool{
	"usr/bin/homenode":                      true,
	"usr/lib/homenode/homenode-supervisor":  true,
	"usr/lib/homenode/homenode-transfer":    true,
	"usr/lib/homenode/homenode-backup":      true,
	"usr/lib/homenode/guest/homenode-guest": true,
}

const checksumInventory = "usr/share/doc/homenode/SHA256SUMS"

func payloadName(raw string) (string, bool) {
	if raw == "./" || raw == "." {
		return ".", true
	}
	name := strings.TrimSuffix(strings.TrimPrefix(raw, "./"), "/")
	if len(name) > 240 || name == "" || path.Clean(name) != name || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || strings.ContainsAny(name, "\n\r\x00") {
		return "", false
	}
	if name == "." || name == "usr" || name == "usr/bin" || name == "usr/lib" || name == "usr/share" || name == "usr/share/doc" {
		return name, true
	}
	for _, prefix := range []string{"usr/lib/homenode", "usr/share/homenode", "usr/share/doc/homenode"} {
		if name == prefix || strings.HasPrefix(name, prefix+"/") {
			return name, true
		}
	}
	return name, packageExecutables[name]
}

// ValidatePayloadArchive inspects uncompressed HomeNode data tar without
// extraction. All regular bytes must match its complete checksum inventory.
// This inventory detects internal consistency; authenticity still comes from
// the independently verified signed package digest and release evidence.
func ValidatePayloadArchive(ctx context.Context, reader io.Reader) error {
	limited := &io.LimitedReader{R: contextualReader{ctx, reader}, N: 2<<30 + 1}
	archive := tar.NewReader(limited)
	seen := map[string]bool{}
	hashes := map[string]string{}
	var inventory []byte
	var total int64
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Join(ErrPackagePayload, err)
		}
		name, ok := payloadName(header.Name)
		if !ok || seen[name] || len(seen) >= 4096 || header.Uid != 0 || header.Gid != 0 || header.Linkname != "" || len(header.PAXRecords) != 0 || len(header.Xattrs) != 0 {
			return ErrPackagePayload
		}
		seen[name] = true
		if header.Typeflag == tar.TypeDir {
			if header.Mode != 0755 || header.Size != 0 {
				return ErrPackagePayload
			}
			continue
		}
		if name == "." || name == "usr" || name == "usr/bin" || name == "usr/lib" || name == "usr/share" || name == "usr/share/doc" || name == "usr/lib/homenode" || name == "usr/share/homenode" || name == "usr/share/doc/homenode" {
			return ErrPackagePayload
		}
		expectedMode := int64(0644)
		if packageExecutables[name] {
			expectedMode = 0755
		}
		if header.Typeflag != tar.TypeReg || header.Mode != expectedMode || header.Size < 0 || header.Size > 256<<20 {
			return ErrPackagePayload
		}
		total += header.Size
		if total > 2<<30 {
			return ErrPackagePayload
		}
		if name == checksumInventory {
			if header.Size > 1<<20 {
				return ErrPackagePayload
			}
			inventory, err = io.ReadAll(archive)
		} else {
			hash := sha256.New()
			var count int64
			count, err = io.Copy(hash, contextualReader{ctx, archive})
			if count != header.Size {
				return ErrPackagePayload
			}
			hashes[name] = hex.EncodeToString(hash.Sum(nil))
		}
		if err != nil {
			return errors.Join(ErrPackagePayload, err)
		}
	}
	for name := range packageExecutables {
		if hashes[name] == "" {
			return ErrPackagePayload
		}
	}
	if hashes["usr/share/homenode/web/index.html"] == "" || len(inventory) == 0 {
		return ErrPackagePayload
	}
	listed := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSuffix(string(inventory), "\n"), "\n") {
		if len(line) < 67 || line[64:66] != "  " {
			return ErrPackagePayload
		}
		name := line[66:]
		if listed[name] || hashes[name] == "" || hashes[name] != line[:64] {
			return ErrPackagePayload
		}
		listed[name] = true
	}
	if len(listed) != len(hashes) {
		return ErrPackagePayload
	}
	// Tar EOF must not conceal a second archive or nonzero trailing content.
	var buffer [4096]byte
	for {
		n, err := limited.Read(buffer[:])
		for _, b := range buffer[:n] {
			if b != 0 {
				return ErrPackagePayload
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Join(ErrPackagePayload, err)
		}
		if limited.N <= 0 {
			return ErrPackagePayload
		}
	}
	if limited.N <= 0 {
		return ErrPackagePayload
	}
	return ctx.Err()
}
