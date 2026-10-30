//go:build linux

package supervisor

import (
	"errors"
	"golang.org/x/sys/unix"
	"testing"
)

type nativeQEMUSocketMetadata struct {
	Domain, Type, Protocol, Listening int
	Local, Peer                       string
	PeerCredentials                   *unix.Ucred `json:",omitempty"`
}

// Explicit disposable-fixture observation only. pidfd_getfd duplicates actual
// authority; keep its lifetime within this helper and perform metadata queries
// only. Never receive messages/SCM_RIGHTS, change shared flags, shutdown, or bind.
func nativeQEMUSocketInventory(t *testing.T, pidfd, number int, expected unix.Stat_t) nativeQEMUSocketMetadata {
	t.Helper()
	fd, err := unix.PidfdGetfd(pidfd, number, 0)
	if err != nil {
		t.Fatal("socket descriptor duplication refused", err)
	}
	defer unix.Close(fd)
	check := func() {
		t.Helper()
		var got unix.Stat_t
		if err := unix.Fstat(fd, &got); err != nil || got.Dev != expected.Dev || got.Ino != expected.Ino || got.Mode != expected.Mode || got.Uid != expected.Uid || got.Gid != expected.Gid || got.Nlink != expected.Nlink {
			t.Fatal("socket descriptor identity changed", err)
		}
	}
	check()
	option := func(name int) int {
		t.Helper()
		value, err := unix.GetsockoptInt(fd, unix.SOL_SOCKET, name)
		if err != nil {
			t.Fatal("socket option unavailable", err)
		}
		return value
	}
	m := nativeQEMUSocketMetadata{Domain: option(unix.SO_DOMAIN), Type: option(unix.SO_TYPE), Protocol: option(unix.SO_PROTOCOL), Listening: option(unix.SO_ACCEPTCONN)}
	if m.Domain != unix.AF_UNIX {
		t.Fatal("unqualified socket family", m.Domain)
	}
	address := func(a unix.Sockaddr, err error) string {
		t.Helper()
		if err != nil {
			t.Fatal("socket address unavailable", err)
		}
		u, ok := a.(*unix.SockaddrUnix)
		if !ok || len(u.Name) > 108 {
			t.Fatal("unqualified Unix socket address")
		}
		return u.Name
	}
	m.Local = address(unix.Getsockname(fd))
	peer, err := unix.Getpeername(fd)
	if err == nil {
		m.Peer = address(peer, nil)
		credentials, err := unix.GetsockoptUcred(fd, unix.SOL_SOCKET, unix.SO_PEERCRED)
		if err != nil || credentials.Pid <= 0 {
			t.Fatal("socket peer credentials unavailable", err)
		}
		m.PeerCredentials = credentials
	} else if !errors.Is(err, unix.ENOTCONN) || m.Listening != 1 {
		t.Fatal("unqualified disconnected socket", err)
	}
	check()
	return m
}
