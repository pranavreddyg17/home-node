//go:build linux

package disktransport

import (
	"golang.org/x/sys/unix"
	"os"
)

// AdmitDisk validates a descriptor received from an authenticated root peer.
// It grants no authority to open a path and does not certify guest consistency.
func AdmitDisk(file *os.File, metadata Metadata) error {
	if metadata.Validate() != nil || !readonly(file) {
		return ErrPacket
	}
	info, err := file.Stat()
	if err != nil || info.Mode().Perm() != 0600 || info.Size() != metadata.Bytes || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return ErrPacket
	}
	var native unix.Stat_t
	if unix.Fstat(int(file.Fd()), &native) != nil || native.Uid != 0 || native.Nlink != 1 {
		return ErrPacket
	}
	return nil
}
