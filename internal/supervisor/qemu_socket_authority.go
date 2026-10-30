package supervisor

import (
	"fmt"
	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"path/filepath"
)

type qemuSocketIdentity struct {
	qemuRegularIdentity
	Family, Type, Protocol, Listening int
	Local, Peer                       string
	PeerPID                           int32
	PeerUID, PeerGID                  uint32
}

func qemuMonitorPath(d Domain, runtimeID uint32) (string, error) {
	if !guestproto.ValidID(d.ID) || runtimeID == 0 || runtimeID > 1<<31-1 {
		return "", ErrPolicy
	}
	short := d.Name()
	if len(short) > 20 {
		short = short[:20]
	}
	return fmt.Sprintf("/var/lib/libvirt/qemu/domain-%d-%s/monitor.sock", runtimeID, short), nil
}

// Candidate roles only. SO_PEERCRED is connection-time identity and cannot
// authenticate the live peer executable or every process holding its endpoint.
func qualifyQEMUSocket(d Domain, runtimeID uint32, s qemuSocketIdentity, access uint32) (string, error) {
	monitor, err := qemuMonitorPath(d, runtimeID)
	if err != nil || d.GuestUID < 65536 || d.GuestUID > 1<<31-1 || d.GuestGID == 0 || d.GuestGID > 1<<31-1 || d.ChannelPath == "/" || len(d.ChannelPath) > 108 || !filepath.IsAbs(d.ChannelPath) || filepath.Clean(d.ChannelPath) != d.ChannelPath || d.ChannelPath == monitor || s.Inode == 0 || s.Links != 1 || s.Mode != 0140000|0777 || s.Family != 1 || s.Type != 1 || s.Protocol != 0 || access != 2 {
		return "", ErrPolicy
	}
	if s.Listening == 1 && s.UID == 0 && s.GID == 0 && s.Peer == "" && s.PeerPID == 0 && s.PeerUID == 0 && s.PeerGID == 0 {
		if s.Local == d.ChannelPath {
			return "candidate-adapter-listener", nil
		}
		if s.Local == monitor {
			return "candidate-monitor-listener", nil
		}
	}
	if s.Listening == 0 && s.Local == monitor && s.Peer == "@" && s.UID == d.GuestUID && s.GID == d.GuestGID && s.PeerPID > 0 && s.PeerUID == 0 && s.PeerGID == 0 {
		return "candidate-monitor-connected", nil
	}
	return "", ErrPolicy
}
