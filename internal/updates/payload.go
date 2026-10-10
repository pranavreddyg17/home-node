package updates

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"path"
	"strings"
)

var ErrPackagePayload = errors.New("package payload is outside HomeNode release policy")

var packageExecutables = map[string]bool{
	"usr/lib/homenode/homenode-gateway":     true,
	"usr/bin/homenode":                      true,
	"usr/lib/homenode/homenode-supervisor":  true,
	"usr/lib/homenode/homenode-transfer":    true,
	"usr/lib/homenode/homenode-inspect":     true,
	"usr/lib/homenode/homenode-backup":      true,
	"usr/lib/homenode/guest/homenode-guest": true,
}

const checksumInventory = "usr/share/doc/homenode/SHA256SUMS"

func payloadName(raw string) (string, bool) {
	if raw == "./" || raw == "." {
		return ".", true
	}
	for _, character := range raw {
		if character < 32 || character == 127 {
			return "", false
		}
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
	directories := map[string]bool{}
	requiredDirectories := map[string]bool{}
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
		if err := ctx.Err(); err != nil {
			return err
		}
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if seen[parent] && !directories[parent] {
				return ErrPackagePayload
			}
			requiredDirectories[parent] = true
		}
		seen[name] = true
		if header.Typeflag == tar.TypeDir {
			if header.Mode != 0755 || header.Size != 0 {
				return ErrPackagePayload
			}
			directories[name] = true
			continue
		}
		if requiredDirectories[name] || strings.HasSuffix(header.Name, "/") {
			return ErrPackagePayload
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
			var prefixLength int64
			if packageExecutables[name] {
				var prefix [64]byte
				if _, err = io.ReadFull(contextualReader{ctx, archive}, prefix[:]); err != nil {
					return errors.Join(ErrPackagePayload, err)
				}
				if !validAMD64ExecutableHeader(prefix[:], header.Size) {
					return ErrPackagePayload
				}
				_, _ = hash.Write(prefix[:])
				prefixLength = 64
			}
			var count int64
			count, err = io.Copy(hash, contextualReader{ctx, archive})
			if count+prefixLength != header.Size {
				return errors.Join(ErrPackagePayload, err)
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

// Header sanity binds executable layout to the supported platform. It does
// not establish executable safety or replace build provenance qualification.
func validAMD64ExecutableHeader(header []byte, size int64) bool {
	if len(header) != 64 || string(header[:4]) != "\x7fELF" || header[4] != 2 || header[5] != 1 || header[6] != 1 {
		return false
	}
	kind := binary.LittleEndian.Uint16(header[16:18])
	if kind != 2 && kind != 3 || binary.LittleEndian.Uint16(header[18:20]) != 62 || binary.LittleEndian.Uint32(header[20:24]) != 1 || binary.LittleEndian.Uint16(header[52:54]) != 64 || binary.LittleEndian.Uint16(header[54:56]) != 56 {
		return false
	}
	programs := uint64(binary.LittleEndian.Uint16(header[56:58]))
	offset := binary.LittleEndian.Uint64(header[32:40])
	return size >= 64 && programs > 0 && programs < 65535 && offset >= 64 && offset <= uint64(size) && programs*56 <= uint64(size)-offset
}
