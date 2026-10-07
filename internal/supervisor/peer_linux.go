//go:build linux

package supervisor

import (
	"golang.org/x/sys/unix"
	"net"
)

func PeerUID(connection *net.UnixConn) (uint32, error) {
	identity, err := PeerProcessIdentity(connection)
	return identity.UID, err
}

func PeerProcessIdentity(connection *net.UnixConn) (UnixPeerIdentity, error) {
	if connection == nil {
		return UnixPeerIdentity{}, ErrPolicy
	}
	raw, err := connection.SyscallConn()
	if err != nil {
		return UnixPeerIdentity{}, err
	}
	var identity UnixPeerIdentity
	var socketErr error
	err = raw.Control(func(fd uintptr) {
		cred, e := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		socketErr = e
		if e == nil {
			identity = UnixPeerIdentity{PID: cred.Pid, UID: cred.Uid, GID: cred.Gid}
		}
	})
	if err != nil {
		return UnixPeerIdentity{}, err
	}
	if socketErr != nil {
		return UnixPeerIdentity{}, socketErr
	}
	if identity.PID <= 0 {
		return UnixPeerIdentity{}, ErrPolicy
	}
	return identity, nil
}
