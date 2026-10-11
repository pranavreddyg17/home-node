//go:build linux

package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

type TLSRetirementInode struct {
	Path   string `json:"path"`
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
	Size   int64  `json:"size"`
}
type gatewayTLSAdmission struct {
	Plan    GatewayTLSRetirementPlan
	Sources []TLSRetirementInode
	files   []*os.File
}

func (a *gatewayTLSAdmission) Close() error {
	var result error
	for _, file := range a.files {
		result = errors.Join(result, file.Close())
	}
	a.files = nil
	return result
}

// Artifact admission only: caller holds e.mu and separately verifies gateway
// identity, activation exclusion and dormancy before committing or applying it.
// Descriptors are retained; TLS contents are hashed through a bounded reader and
// never included in the plan. Hardlinks, ACLs and replaced paths are refused.
func (e *Engine) admitGatewayTLSRetirementLocked(ctx context.Context, identity GatewayAccount) (result *gatewayTLSAdmission, resultErr error) {
	admission := &gatewayTLSAdmission{}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, admission.Close())
		}
	}()
	installed, err := e.load()
	if err != nil {
		return nil, err
	}
	for _, item := range installed.Items {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := e.matches(item); err != nil {
			return nil, err
		}
	}
	metadata := []ConfigurationRemovalFile{}
	for _, name := range []string{"etc/homenode/tls/server.key", "etc/homenode/tls/server.crt", "etc/homenode/tls"} {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := e.parents(name); err != nil {
			return nil, err
		}
		file, err := e.host.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			return nil, err
		}
		admission.files = append(admission.files, file)
		info, err := file.Stat()
		if err != nil {
			return nil, err
		}
		var stat unix.Stat_t
		if unix.Fstat(int(file.Fd()), &stat) != nil || stat.Uid != 0 || stat.Gid > 1<<31-1 || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
			return nil, ErrConflict
		}
		directory := name == "etc/homenode/tls"
		if info.IsDir() != directory || (!directory && (!info.Mode().IsRegular() || stat.Nlink != 1 || info.Size() <= 0 || info.Size() > 16<<10)) {
			return nil, ErrConflict
		}
		attributes := []string{"system.posix_acl_access"}
		if directory {
			attributes = append(attributes, "system.posix_acl_default")
		}
		for _, attribute := range attributes {
			if _, err := unix.Fgetxattr(int(file.Fd()), attribute, nil); !errors.Is(err, unix.ENODATA) {
				return nil, ErrConflict
			}
		}
		named, err := e.host.Lstat(name)
		if err != nil || !os.SameFile(info, named) {
			return nil, ErrConflict
		}
		admission.Sources = append(admission.Sources, TLSRetirementInode{Path: name, Device: uint64(stat.Dev), Inode: stat.Ino, Size: info.Size()})
		if !directory {
			hash := sha256.New()
			size, err := io.Copy(hash, io.LimitReader(contextReader{ctx, file}, (16<<10)+1))
			if err != nil || size != info.Size() {
				return nil, ErrConflict
			}
			metadata = append(metadata, ConfigurationRemovalFile{Path: name, UID: 0, GID: int(stat.Gid), Mode: uint32(info.Mode().Perm()), SHA256: hex.EncodeToString(hash.Sum(nil))})
		}
	}
	plan, err := gatewayTLSRetirementPlan(installed, identity, metadata[0], metadata[1])
	if err != nil {
		return nil, err
	}
	admission.Plan = plan
	if err := e.verifyGatewayTLSAdmissionLocked(ctx, admission); err != nil {
		return nil, err
	}
	return admission, nil
}

// Verify again before intent publication; an open descriptor alone does not
// establish unchanged contents, permissions, ACLs or the current named inode.
func (e *Engine) verifyGatewayTLSAdmissionLocked(ctx context.Context, a *gatewayTLSAdmission) error {
	if len(a.files) != 3 || len(a.Sources) != 3 {
		return ErrConflict
	}
	expected := []ConfigurationRemovalFile{a.Plan.Key, a.Plan.Certificate, {Path: "etc/homenode/tls", UID: 0, GID: a.Plan.Identity.GID, Mode: 0750}}
	for index, file := range a.files {
		if err := ctx.Err(); err != nil {
			return err
		}
		var stat unix.Stat_t
		if unix.Fstat(int(file.Fd()), &stat) != nil || uint64(stat.Dev) != a.Sources[index].Device || stat.Ino != a.Sources[index].Inode || stat.Uid != 0 || int(stat.Gid) != expected[index].GID || stat.Mode&07777 != expected[index].Mode {
			return ErrConflict
		}
		info, err := file.Stat()
		named, nameErr := e.host.Lstat(expected[index].Path)
		if err != nil || nameErr != nil || !os.SameFile(info, named) || a.Sources[index].Path != expected[index].Path || info.IsDir() != (index == 2) {
			return ErrConflict
		}
		attributes := []string{"system.posix_acl_access"}
		if index == 2 {
			attributes = append(attributes, "system.posix_acl_default")
		}
		for _, attribute := range attributes {
			if _, err := unix.Fgetxattr(int(file.Fd()), attribute, nil); !errors.Is(err, unix.ENODATA) {
				return ErrConflict
			}
		}
		if index != 2 {
			if stat.Nlink != 1 || info.Size() != a.Sources[index].Size || info.Size() <= 0 || info.Size() > 16<<10 {
				return ErrConflict
			}
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				return err
			}
			hash := sha256.New()
			size, err := io.Copy(hash, io.LimitReader(contextReader{ctx, file}, (16<<10)+1))
			if err != nil || size != info.Size() || hex.EncodeToString(hash.Sum(nil)) != expected[index].SHA256 {
				return ErrConflict
			}
		}
	}
	return ctx.Err()
}
