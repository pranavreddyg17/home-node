package supervisor

import "github.com/pranavreddyg17/home-node/internal/guestproto"

// Device metadata only. An admitted device node does not qualify its ioctl
// surface, QEMU seccomp policy, or descriptor-transfer lifetime.
type qemuDeviceIdentity struct {
	qemuRegularIdentity
	Major, Minor uint32
}

func qualifyQEMUDevice(d Domain, target string, open, named qemuDeviceIdentity, access uint32) (string, error) {
	if !guestproto.ValidID(d.ID) || d.GuestUID < 65536 || d.GuestUID > 1<<31-1 || d.GuestGID == 0 || d.GuestGID > 1<<31-1 || open != named || open.Inode == 0 || open.Links != 1 || open.UID != 0 {
		return "", ErrPolicy
	}
	switch target {
	case "/dev/null":
		if open.GID == 0 && open.Mode == 0020000|0666 && open.Major == 1 && open.Minor == 3 && access == 2 {
			return "null-device", nil
		}
	case "/dev/urandom":
		if open.GID == 0 && open.Mode == 0020000|0666 && open.Major == 1 && open.Minor == 9 && access == 0 {
			return "random-device", nil
		}
	case "/dev/kvm":
		if open.GID == d.GuestGID && open.Mode == 0020000|0660 && open.Major == 10 && open.Minor == 232 && access == 2 {
			return "kvm-device", nil
		}
	}
	return "", ErrPolicy
}
