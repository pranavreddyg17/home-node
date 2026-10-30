package supervisor

import (
	"path/filepath"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// Linux regular-file identity and access mode, observed independently from an
// open descriptor and its protected named path. No data or key bytes are read.
type qemuRegularIdentity struct {
	Device, Inode, Links uint64
	UID, GID, Mode       uint32
}

// Metadata qualification only: the candidate PID role still needs explicit
// inherited-authority and lifetime admission. Never adopt arbitrary root-owned
// files, deleted handles, logs or aliases just because they were observed.
func qualifyQEMURegularWrite(d Domain, target string, open, named qemuRegularIdentity, access uint32) (string, error) {
	if !guestproto.ValidID(d.ID) || d.GuestUID < 65536 || d.GuestUID > 1<<31-1 || d.GuestGID == 0 || d.GuestGID > 1<<31-1 || !filepath.IsAbs(target) || filepath.Clean(target) != target || open != named || open.Inode == 0 || open.Links != 1 {
		return "", ErrPolicy
	}
	if d.DataPath == filepath.Join("/run/libvirt/qemu", d.Name()+".pid") {
		return "", ErrPolicy
	}
	switch target {
	case d.DataPath:
		if !filepath.IsAbs(d.DataPath) || filepath.Clean(d.DataPath) != d.DataPath || access != 2 || open.UID != d.GuestUID || open.GID != d.GuestGID || open.Mode != 0100000|0600 {
			return "", ErrPolicy
		}
		return "managed-data", nil
	case filepath.Join("/run/libvirt/qemu", d.Name()+".pid"):
		if access != 1 || open.UID != 0 || open.GID != 0 || open.Mode != 0100000|0644 {
			return "", ErrPolicy
		}
		return "candidate-domain-pidfile", nil
	default:
		return "", ErrPolicy
	}
}
