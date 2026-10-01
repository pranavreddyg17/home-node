//go:build linux

package socketactivation

import (
	"net"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// TakePrivateListener consumes exactly one named systemd activation descriptor.
// It requires a root-owned, backup-group-only filesystem socket and a root-owned
// parent inaccessible to unprivileged writers. The caller owns the returned
// listener. This is not an alternative to per-request kernel peer authentication.
func TakePrivateListener(name, path string, backupGID uint32) (net.Listener, error) {
	valid := validEnvironment(os.Getenv, os.Getpid(), name)
	for _, key := range []string{"LISTEN_PID", "LISTEN_FDS", "LISTEN_FDNAMES", "LISTEN_PIDFDID"} {
		_ = os.Unsetenv(key)
	}
	if !valid || backupGID == 0 || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, ErrListener
	}
	var node, parent unix.Stat_t
	if unix.Lstat(path, &node) != nil || node.Mode&unix.S_IFMT != unix.S_IFSOCK || node.Uid != 0 || node.Gid != backupGID || node.Mode&07777 != 0660 {
		return nil, ErrListener
	}
	if unix.Lstat(filepath.Dir(path), &parent) != nil || parent.Mode&unix.S_IFMT != unix.S_IFDIR || parent.Uid != 0 || parent.Mode&0022 != 0 {
		return nil, ErrListener
	}
	kind, err := unix.GetsockoptInt(3, unix.SOL_SOCKET, unix.SO_TYPE)
	if err != nil || kind != unix.SOCK_STREAM {
		return nil, ErrListener
	}
	accepting, err := unix.GetsockoptInt(3, unix.SOL_SOCKET, unix.SO_ACCEPTCONN)
	if err != nil || accepting != 1 {
		return nil, ErrListener
	}
	address, err := unix.Getsockname(3)
	if err != nil {
		return nil, ErrListener
	}
	local, ok := address.(*unix.SockaddrUnix)
	if !ok || local.Name != path {
		return nil, ErrListener
	}
	unix.CloseOnExec(3)
	file := os.NewFile(3, "activated-private-listener")
	if file == nil {
		return nil, ErrListener
	}
	defer file.Close()
	listener, err := net.FileListener(file)
	if err != nil {
		return nil, ErrListener
	}
	if _, ok := listener.(*net.UnixListener); !ok {
		listener.Close()
		return nil, ErrListener
	}
	return listener, nil
}
