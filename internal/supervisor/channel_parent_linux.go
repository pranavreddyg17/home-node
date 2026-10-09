//go:build linux

package supervisor

import "golang.org/x/sys/unix"

// The transfer identity traverses the parent; only root may create entries.
func (m *Manager) reservedChannelParentAdmitted(metadata unix.Stat_t) bool {
	if m == nil {
		return false
	}
	gid, err := m.channelAccessGID()
	return err == nil && metadata.Uid == 0 && metadata.Gid == gid && metadata.Mode == unix.S_IFDIR|0710
}
