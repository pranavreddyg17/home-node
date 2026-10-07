package supervisor

import (
	"context"
	"errors"
	"io"
	"os"
	"runtime"
	"syscall"
)

// ObserveLocalGuestUIDConflicts reads local account/delegation files only.
// It does not prove NSS eligibility, absence of running processes or exclusive
// pool provisioning, and therefore cannot alone authorize a guest launch.
func ObserveLocalGuestUIDConflicts(ctx context.Context, pool GuestUIDPool) (observed GuestUIDPool, result error) {
	if err := ctx.Err(); err != nil {
		return observed, err
	}
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || pool.validate() != nil {
		return observed, ErrPolicy
	}
	before, err := os.Lstat("/etc")
	if err != nil || !before.IsDir() || before.Mode().Perm()&0022 != 0 {
		return observed, ErrPolicy
	}
	root, err := os.OpenRoot("/etc")
	if err != nil {
		return observed, err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	opened, err := root.Stat(".")
	stat, ok := openedSysUID(opened)
	if err != nil || !os.SameFile(before, opened) || !ok || stat != 0 || opened.Mode().Perm()&0022 != 0 {
		return observed, ErrPolicy
	}
	passwd, err := readGuestUIDAccountFile(ctx, root, "passwd", 0)
	if err != nil {
		return observed, err
	}
	subuid, err := readGuestUIDAccountFile(ctx, root, "subuid", 0)
	if err != nil {
		return observed, err
	}
	current, err := os.Lstat("/etc")
	currentUID, currentOK := openedSysUID(current)
	if err != nil || !os.SameFile(opened, current) || !currentOK || currentUID != 0 || current.Mode().Perm()&0022 != 0 {
		return observed, ErrPolicy
	}
	return accountGuestUIDConflicts(ctx, pool, passwd, subuid)
}

func openedSysUID(info os.FileInfo) (uint32, bool) {
	if info == nil {
		return 0, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return stat.Uid, true
}

func readGuestUIDAccountFile(ctx context.Context, root *os.Root, name string, owner uint32) (data []byte, result error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if root == nil || (name != "passwd" && name != "subuid" && name != "nsswitch.conf" && name != "login.defs") {
		return nil, ErrPolicy
	}
	before, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	uid, ok := openedSysUID(before)
	if !ok || uid != owner || !before.Mode().IsRegular() || before.Mode().Perm()&0022 != 0 || before.Size() < 0 || before.Size() > 1<<20 {
		return nil, ErrPolicy
	}
	file, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	opened, err := file.Stat()
	openedUID, openedOK := openedSysUID(opened)
	if err != nil || !os.SameFile(before, opened) || !openedOK || openedUID != owner || opened.Mode() != before.Mode() {
		return nil, ErrPolicy
	}
	data, err = io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	current, err := root.Lstat(name)
	currentUID, currentOK := openedSysUID(current)
	if err != nil || !currentOK || currentUID != owner || !os.SameFile(opened, current) || len(data) > 1<<20 || int64(len(data)) != before.Size() || current.Size() != before.Size() || current.Mode() != before.Mode() {
		return nil, ErrPolicy
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return data, nil
}

// ObserveGuestUIDNameServiceEligibility verifies a protected local-only NSS
// configuration. It does not provision or reserve the guest UID pool.
func ObserveGuestUIDNameServiceEligibility(ctx context.Context) error {
	return observeGuestUIDNameServiceEligibilityAt(ctx, "/etc")
}

func observeGuestUIDNameServiceEligibilityAt(ctx context.Context, directory string) error {
	return observeProtectedGuestIdentityConfig(ctx, directory, "nsswitch.conf", func(data []byte) error { return validateGuestUIDNameServices(ctx, data) })
}

func observeProtectedGuestIdentityConfig(ctx context.Context, directory, name string, validate func([]byte) error) (result error) {
	if validate == nil {
		return ErrPolicy
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return ErrPolicy
	}
	before, err := os.Lstat(directory)
	owner, ok := openedSysUID(before)
	if err != nil || !ok || owner != 0 || !before.IsDir() || before.Mode().Perm()&0022 != 0 {
		return ErrPolicy
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	opened, err := root.Stat(".")
	owner, ok = openedSysUID(opened)
	if err != nil || !ok || owner != 0 || !os.SameFile(before, opened) || opened.Mode().Perm()&0022 != 0 {
		return ErrPolicy
	}
	data, err := readGuestUIDAccountFile(ctx, root, name, 0)
	if err != nil {
		return err
	}
	if err := validate(data); err != nil {
		return err
	}
	current, err := os.Lstat(directory)
	owner, ok = openedSysUID(current)
	if err != nil || !ok || owner != 0 || !os.SameFile(opened, current) || current.Mode().Perm()&0022 != 0 {
		return ErrPolicy
	}
	return ctx.Err()
}
