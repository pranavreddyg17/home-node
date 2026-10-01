//go:build linux

package runtimeclient

import (
	"github.com/pranavreddyg17/home-node/internal/backup"
	"golang.org/x/sys/unix"
	"path/filepath"
)

func validateBackupCredentialSocket(path string, controllerGID uint32) error {
	var node, parent unix.Stat_t
	if unix.Lstat(path, &node) != nil || node.Mode&unix.S_IFMT != unix.S_IFSOCK || node.Uid != 0 || node.Gid != controllerGID || node.Mode&07777 != 0660 {
		return backup.ErrManifest
	}
	if unix.Lstat(filepath.Dir(path), &parent) != nil || parent.Mode&unix.S_IFMT != unix.S_IFDIR || parent.Uid != 0 || parent.Mode&0022 != 0 {
		return backup.ErrManifest
	}
	return nil
}
