//go:build linux

package supervisor

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

// grantGuestChannelAccess changes the pinned socket inode, never a pathname
// resolved again for mutation. Kernels without descriptor chmod support refuse.
func grantGuestChannelAccess(ctx context.Context, path string, uid uint32, gid int) (result error) {
	return grantGuestChannelAccessWithPeer(ctx, path, uid, gid, nil)
}

// Only the verified libvirt domain path may adopt its root-created listener.
func grantLibvirtGuestChannelAccess(ctx context.Context, path string, uid uint32, gid int, peer UnixPeerIdentity) error {
	if peer.PID <= 0 || peer.UID != uid || peer.UID == 0 || peer.GID == 0 {
		return ErrPolicy
	}
	return grantGuestChannelAccessWithPeer(ctx, path, uid, gid, &peer)
}

func grantGuestChannelAccessWithPeer(ctx context.Context, path string, uid uint32, gid int, peer *UnixPeerIdentity) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if os.Geteuid() != 0 || uid == 0 || uid > 1<<31-1 || gid < 1 || gid > 1<<31-1 || !filepath.IsAbs(path) || filepath.Clean(path) != path || filepath.Base(path) != "adapter.sock" {
		return ErrPolicy
	}
	parentPath := filepath.Dir(path)
	before, err := os.Lstat(parentPath)
	owner, ok := openedSysUID(before)
	if err != nil || !ok || owner != uid || !before.IsDir() || before.Mode().Perm() != 0710 || before.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return ErrPolicy
	}
	root, err := os.OpenRoot(parentPath)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	parent, err := root.Open(".")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, parent.Close()) }()
	opened, err := parent.Stat()
	owner, ok = openedSysUID(opened)
	var directory unix.Stat_t
	if err != nil || !ok || owner != uid || !os.SameFile(before, opened) || opened.Mode().Perm() != 0710 || opened.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || unix.Fstat(int(parent.Fd()), &directory) != nil || directory.Gid != uint32(gid) {
		return ErrPolicy
	}
	file, err := root.OpenFile("adapter.sock", unix.O_PATH|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, file.Close()) }()
	var socket unix.Stat_t
	if unix.Fstat(int(file.Fd()), &socket) != nil || socket.Mode&unix.S_IFMT != unix.S_IFSOCK || (socket.Uid != uid && !(peer != nil && socket.Uid == 0 && socket.Gid == 0 && socket.Mode&07777 == 0775)) || socket.Nlink != 1 {
		return ErrPolicy
	}
	original, err := file.Stat()
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Probe only initial root ownership. Audit of an already adopted socket
	// must not queue extra connections behind the active adapter stream.
	if peer != nil && socket.Uid == 0 {
		connection, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", path)
		if err != nil {
			return err
		}
		identity, identityErr := PeerProcessIdentity(connection.(*net.UnixConn))
		closeErr := connection.Close()
		if identityErr != nil || closeErr != nil || identity != *peer {
			return errors.Join(ErrPolicy, identityErr, closeErr)
		}
		current, err := root.Lstat("adapter.sock")
		if err != nil || !os.SameFile(original, current) {
			return ErrPolicy
		}
	}
	// Peer observation may block. Re-admit both pinned objects before mutation,
	// including link count and ownership changes during that interval.
	var admitted unix.Stat_t
	currentSocket, err := root.Lstat("adapter.sock")
	if err != nil || !os.SameFile(original, currentSocket) || unix.Fstat(int(file.Fd()), &admitted) != nil || admitted.Dev != socket.Dev || admitted.Ino != socket.Ino || admitted.Mode != socket.Mode || admitted.Uid != socket.Uid || admitted.Gid != socket.Gid || admitted.Nlink != 1 {
		return ErrPolicy
	}
	currentDirectory, err := os.Lstat(parentPath)
	if err != nil || !os.SameFile(opened, currentDirectory) || currentDirectory.Mode() != opened.Mode() || unix.Fstat(int(parent.Fd()), &directory) != nil || directory.Uid != uid || directory.Gid != uint32(gid) {
		return ErrPolicy
	}
	ownerUID := -1
	if socket.Uid == 0 {
		ownerUID = int(uid)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := unix.Fchownat(int(file.Fd()), "", ownerUID, gid, unix.AT_EMPTY_PATH|unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := unix.Fchmodat(int(file.Fd()), "", 0660, unix.AT_EMPTY_PATH); err != nil {
		return err
	}
	var final unix.Stat_t
	current, err := root.Lstat("adapter.sock")
	if err != nil || !os.SameFile(original, current) || unix.Fstat(int(file.Fd()), &final) != nil || final.Dev != socket.Dev || final.Ino != socket.Ino || final.Uid != uid || final.Gid != uint32(gid) || final.Nlink != 1 || final.Mode&07777 != 0660 {
		return ErrPolicy
	}
	currentParent, err := os.Lstat(parentPath)
	owner, ok = openedSysUID(currentParent)
	if err != nil || !ok || owner != uid || !os.SameFile(opened, currentParent) || currentParent.Mode() != opened.Mode() || unix.Fstat(int(parent.Fd()), &directory) != nil || directory.Uid != uid || directory.Gid != uint32(gid) {
		return ErrPolicy
	}
	return ctx.Err()
}
