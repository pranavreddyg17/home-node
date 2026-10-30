//go:build linux

package supervisor

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

type qemuSocketMetadata struct {
	Domain, Type, Protocol, Listening int
	Local, Peer                       string
	PeerCredentials                   *unix.Ucred `json:",omitempty"`
}

// observeQEMUSocketMetadata briefly duplicates actual socket authority. The
// caller must independently admit the process/pidfd and bound the source slot.
// No messages, flags or connection state may be changed. A returned observation
// is not peer authorization or retained exclusion of descriptor transfer.
func observeQEMUSocketMetadata(pidfd, number int, expected unix.Stat_t) (metadata qemuSocketMetadata, result error) {
	fd, err := unix.PidfdGetfd(pidfd, number, 0)
	if err != nil {
		return metadata, fmt.Errorf("duplicate socket observation: %w", err)
	}
	defer func() { result = errors.Join(result, unix.Close(fd)) }()
	return readQEMUSocketMetadata(fd, expected)
}

func readQEMUSocketMetadata(fd int, expected unix.Stat_t) (qemuSocketMetadata, error) {
	var empty qemuSocketMetadata
	check := func() error {
		var got unix.Stat_t
		if err := unix.Fstat(fd, &got); err != nil {
			return err
		}
		if got.Mode&unix.S_IFMT != unix.S_IFSOCK || got.Dev != expected.Dev || got.Ino != expected.Ino || got.Mode != expected.Mode || got.Uid != expected.Uid || got.Gid != expected.Gid || got.Nlink != expected.Nlink {
			return ErrPolicy
		}
		return nil
	}
	if err := check(); err != nil {
		return empty, err
	}
	values := make([]int, 4)
	for i, option := range []int{unix.SO_DOMAIN, unix.SO_TYPE, unix.SO_PROTOCOL, unix.SO_ACCEPTCONN} {
		value, err := unix.GetsockoptInt(fd, unix.SOL_SOCKET, option)
		if err != nil {
			return empty, err
		}
		values[i] = value
	}
	m := qemuSocketMetadata{Domain: values[0], Type: values[1], Protocol: values[2], Listening: values[3]}
	if m.Domain != unix.AF_UNIX {
		return empty, ErrPolicy
	}
	address := func(a unix.Sockaddr, err error) (string, error) {
		if err != nil {
			return "", err
		}
		u, ok := a.(*unix.SockaddrUnix)
		if !ok || len(u.Name) > 108 {
			return "", ErrPolicy
		}
		return u.Name, nil
	}
	var err error
	m.Local, err = address(unix.Getsockname(fd))
	if err != nil {
		return empty, err
	}
	peer, err := unix.Getpeername(fd)
	if err == nil {
		m.Peer, err = address(peer, nil)
		if err != nil {
			return empty, err
		}
		credentials, err := unix.GetsockoptUcred(fd, unix.SOL_SOCKET, unix.SO_PEERCRED)
		if err != nil {
			return empty, err
		}
		if credentials.Pid <= 0 {
			return empty, ErrPolicy
		}
		m.PeerCredentials = credentials
	} else if !errors.Is(err, unix.ENOTCONN) || m.Listening != 1 {
		return empty, ErrPolicy
	}
	if err := check(); err != nil {
		return empty, err
	}
	return m, nil
}
