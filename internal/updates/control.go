package updates

import (
	"archive/tar"
	"bufio"
	"errors"
	"io"
	"strings"
)

var ErrPackageControl = errors.New("package control archive is outside HomeNode release policy")

const homeNodeDependencies = "libvirt-daemon-system, libvirt-clients, qemu-system-x86, qemu-utils, e2fsprogs, apparmor, restic"

// ValidateControlArchive inspects decompressed Debian control tar bytes without
// extraction or execution. This is a content gate, not complete Debian package
// validation: callers must independently verify the outer archive and payload.
// The supported package format deliberately has no maintainer scripts/triggers.
func ValidateControlArchive(reader io.Reader, release ReleaseMetadata) error {
	if !releaseName.MatchString(release.Release) || release.Platform != "ubuntu-24.04-amd64" {
		return ErrPackageControl
	}
	limited := &io.LimitedReader{R: reader, N: 1<<20 + 1}
	archive := tar.NewReader(limited)
	found := false
	entries := 0
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil || header == nil {
			return ErrPackageControl
		}
		entries++
		if entries > 2 || header.Uid != 0 || header.Gid != 0 || header.Linkname != "" || len(header.PAXRecords) != 0 || len(header.Xattrs) != 0 {
			return ErrPackageControl
		}
		if header.Name == "./" || header.Name == "." {
			if header.Typeflag != tar.TypeDir || header.Size != 0 || header.Mode != 0755 {
				return ErrPackageControl
			}
			continue
		}
		if found || header.Name != "./control" && header.Name != "control" || header.Typeflag != tar.TypeReg || header.Mode != 0644 || header.Size < 1 || header.Size > 64<<10 {
			return ErrPackageControl
		}
		data, err := io.ReadAll(io.LimitReader(archive, header.Size+1))
		if err != nil || int64(len(data)) != header.Size || validateControlFields(string(data), release.Release) != nil {
			return ErrPackageControl
		}
		found = true
	}
	// Consume bounded trailing padding; reject nonzero trailing archive content.
	for {
		var buffer [4096]byte
		n, err := limited.Read(buffer[:])
		for _, b := range buffer[:n] {
			if b != 0 {
				return ErrPackageControl
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil || limited.N == 0 {
			return ErrPackageControl
		}
	}
	if !found || limited.N <= 0 {
		return ErrPackageControl
	}
	return nil
}

func validateControlFields(data, version string) error {
	if strings.ContainsAny(data, "\x00\r") {
		return ErrPackageControl
	}
	allowed := map[string]bool{"Package": true, "Version": true, "Section": true, "Priority": true, "Architecture": true, "Maintainer": true, "Depends": true, "Description": true, "Installed-Size": true, "Homepage": true}
	fields := map[string]string{}
	last := ""
	ended := false
	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			ended = true
			continue
		}
		if ended {
			return ErrPackageControl
		}
		if line[0] == ' ' || line[0] == '\t' {
			if last != "Description" {
				return ErrPackageControl
			}
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok || !allowed[name] || fields[name] != "" || strings.TrimSpace(value) == "" {
			return ErrPackageControl
		}
		fields[name] = strings.TrimSpace(value)
		last = name
	}
	if scanner.Err() != nil || fields["Package"] != "homenode" || fields["Version"] != version || fields["Architecture"] != "amd64" || fields["Depends"] != homeNodeDependencies || fields["Maintainer"] == "" || fields["Description"] == "" {
		return ErrPackageControl
	}
	return nil
}
