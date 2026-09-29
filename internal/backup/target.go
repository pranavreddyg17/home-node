// Package backup owns backup destination admission. Backup/restore orchestration
// must use a pinned admitted directory; a missing drive is never a host path.
package backup

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var ErrTarget = errors.New("registered backup drive is unavailable or mismatched")
var uuidPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,63}$`)
var repositoryPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Target struct {
	MountPath    string `json:"mountPath"`
	UUID         string `json:"uuid"`
	RepositoryID string `json:"repositoryId"`
}

type Device struct{ Major, Minor uint32 }
type Mount struct {
	Device                 Device
	Root, Path, Filesystem string
	Writable               bool
}

func (t Target) Validate() error {
	if !filepath.IsAbs(t.MountPath) || filepath.Clean(t.MountPath) != t.MountPath || t.MountPath == "/" || !uuidPattern.MatchString(t.UUID) || !repositoryPattern.MatchString(t.RepositoryID) {
		return ErrTarget
	}
	return nil
}

// AdmitMount requires an exact, writable filesystem mount with the registered
// device identity. A bind of a subdirectory or the host root disk is rejected.
func AdmitMount(t Target, mounts []Mount, registered Device) (Mount, error) {
	if err := t.Validate(); err != nil {
		return Mount{}, err
	}
	var root Device
	var hasRoot bool
	for _, m := range mounts {
		if m.Path == "/" {
			root = m.Device
			hasRoot = true
		}
	}
	if !hasRoot || root == registered {
		return Mount{}, ErrTarget
	}
	var result Mount
	count := 0
	for _, m := range mounts {
		if m.Path != t.MountPath {
			continue
		}
		count++
		if m.Device != registered || m.Root != "/" || !m.Writable {
			return Mount{}, ErrTarget
		}
		switch m.Filesystem {
		case "ext4", "xfs", "btrfs":
		default:
			return Mount{}, ErrTarget
		}
		result = m
	}
	if count != 1 {
		return Mount{}, ErrTarget
	}
	return result, nil
}

func mountField(s string) (string, error) {
	var result strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			result.WriteByte(s[i])
			continue
		}
		if i+3 >= len(s) {
			return "", ErrTarget
		}
		code := s[i+1 : i+4]
		switch code {
		case "040":
			result.WriteByte(' ')
		case "011":
			result.WriteByte('\t')
		case "012":
			result.WriteByte('\n')
		case "134":
			result.WriteByte('\\')
		default:
			return "", ErrTarget
		}
		i += 3
	}
	return result.String(), nil
}

// ParseMountInfo accepts the kernel mountinfo format, not shell mount output.
func ParseMountInfo(data []byte) ([]Mount, error) {
	if len(data) > 4<<20 {
		return nil, ErrTarget
	}
	mounts := []Mount{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.Fields(line)
		separator := -1
		for i, f := range fields {
			if f == "-" {
				separator = i
				break
			}
		}
		if separator < 6 || len(fields) != separator+4 {
			return nil, ErrTarget
		}
		numbers := strings.Split(fields[2], ":")
		if len(numbers) != 2 {
			return nil, ErrTarget
		}
		major, err := strconv.ParseUint(numbers[0], 10, 32)
		if err != nil {
			return nil, ErrTarget
		}
		minor, err := strconv.ParseUint(numbers[1], 10, 32)
		if err != nil {
			return nil, ErrTarget
		}
		root, err := mountField(fields[3])
		if err != nil {
			return nil, err
		}
		path, err := mountField(fields[4])
		if err != nil {
			return nil, err
		}
		rw, ro := false, false
		for _, option := range strings.Split(fields[5], ",") {
			rw = rw || option == "rw"
			ro = ro || option == "ro"
		}
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return nil, fmt.Errorf("invalid mount path: %w", ErrTarget)
		}
		mounts = append(mounts, Mount{Device: Device{uint32(major), uint32(minor)}, Root: root, Path: path, Filesystem: fields[separator+1], Writable: rw && !ro})
	}
	return mounts, nil
}
