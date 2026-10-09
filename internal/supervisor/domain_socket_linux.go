//go:build linux

package supervisor

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// verifyReservedDomainSocket is called under the manager's runtime exclusion.
// It joins live domain confinement to durable socket provenance. Existing
// receipts are audited without queuing another adapter connection.
func (m *Manager) verifyReservedDomainSocket(ctx context.Context, d Domain, revision int64) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	if m == nil || m.Store == nil || m.Backend == nil || m.GuestUIDPool == nil || os.Geteuid() != 0 || revision < 1 || d.ChannelPath != filepath.Join(m.Channels, d.ID, "adapter.sock") {
		return ErrPolicy
	}
	switch b := m.Backend.(type) {
	case LinuxBackend:
	case *LinuxBackend:
		if b == nil {
			return ErrPolicy
		}
	default:
		return ErrPolicy
	}
	if err := m.Backend.Verify(ctx, d); err != nil {
		return err
	}
	var existing int
	err := m.Store.DB.QueryRowContext(ctx, `SELECT 1 FROM runtime_channel_sockets WHERE instance_id=? AND revision=?`, d.ID, revision).Scan(&existing)
	create := errors.Is(err, sql.ErrNoRows)
	var channel ChannelOwnershipIntent
	if create {
		channel, err = m.loadChannelOwnershipIntent(ctx, d)
	} else if err == nil {
		var saved ChannelSocketIntent
		saved, err = m.loadActiveChannelSocketIntent(ctx, d, revision)
		channel = saved.Channel
	}
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(filepath.Dir(d.ChannelPath))
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, directory.Close()) }()
	socket, err := root.OpenFile("adapter.sock", unix.O_PATH|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, socket.Close()) }()
	var stat unix.Stat_t
	if unix.Fstat(int(socket.Fd()), &stat) != nil {
		return ErrPolicy
	}
	intent := ChannelSocketIntent{Channel: channel, Revision: revision, Device: uint64(stat.Dev), Inode: stat.Ino}
	checkPath := func() error {
		if !samePathMount(int(directory.Fd()), unix.AT_FDCWD, filepath.Dir(d.ChannelPath)) {
			return ErrPolicy
		}
		opened, e := directory.Stat()
		if e != nil {
			return e
		}
		named, e := os.Lstat(filepath.Dir(d.ChannelPath))
		if e != nil || !os.SameFile(opened, named) {
			return errors.Join(ErrPolicy, e)
		}
		return ctx.Err()
	}
	if err := checkPath(); err != nil {
		return err
	}
	if !create {
		if err := m.verifyChannelSocketIntent(ctx, intent); err != nil {
			return err
		}
	} else {
		readPID := func() (int, error) {
			data, e := os.ReadFile(filepath.Join("/run/libvirt/qemu", d.Name()+".pid"))
			if e != nil {
				return 0, e
			}
			pid, e := strconv.Atoi(strings.TrimSpace(string(data)))
			if e != nil || pid < 1 || pid > math.MaxInt32 {
				return 0, ErrPolicy
			}
			return pid, verifyGuestDACProcess(ctx, pid, d.GuestUID, d.GuestGID)
		}
		pid, err := readPID()
		if err != nil {
			return err
		}
		connection, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", d.ChannelPath)
		if err != nil {
			return err
		}
		peer, peerErr := PeerProcessIdentity(connection.(*net.UnixConn))
		closeErr := connection.Close()
		if peerErr != nil || closeErr != nil || peer != (UnixPeerIdentity{PID: int32(pid), UID: d.GuestUID, GID: d.GuestGID}) {
			return errors.Join(ErrPolicy, peerErr, closeErr)
		}
		currentPID, err := readPID()
		if err != nil || currentPID != pid {
			return errors.Join(ErrPolicy, err)
		}
	}
	if err := checkPath(); err != nil {
		return err
	}
	// Recheck confinement after the potentially blocking peer observation.
	if err := m.Backend.Verify(ctx, d); err != nil {
		return err
	}
	actual, err := m.checkPinnedChannelSocket(ctx, revision, channel, directory, socket, create)
	if err != nil {
		return err
	}
	if actual != intent {
		return ErrPolicy
	}
	return checkPath()
}
