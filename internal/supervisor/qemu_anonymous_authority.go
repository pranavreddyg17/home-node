package supervisor

import (
	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"strconv"
	"strings"
)

// Candidate metadata for the supported Linux/QEMU fixture. Anonymous inode
// names do not attest VM ownership, ioctl safety or absence of FD transfer.
func qualifyQEMUAnonymous(d Domain, target string, s qemuRegularIdentity, access uint32) (string, error) {
	if !guestproto.ValidID(d.ID) || d.GuestUID < 65536 || d.GuestUID > 1<<31-1 || d.GuestGID == 0 || d.GuestGID > 1<<31-1 || d.Image.VCPUs < 1 || d.Image.VCPUs > 256 || s.Device == 0 || s.Inode == 0 || s.Links != 1 || s.UID != 0 || s.GID != 0 || s.Mode != 0600 {
		return "", ErrPolicy
	}
	if access == 2 {
		switch target {
		case "anon_inode:[eventfd]":
			return "candidate-eventfd", nil
		case "anon_inode:[signalfd]":
			return "candidate-signalfd", nil
		case "anon_inode:kvm-vm":
			return "candidate-kvm-vm", nil
		}
	}
	for _, kind := range []struct {
		prefix, role string
		access       uint32
	}{
		{"anon_inode:kvm-vcpu:", "candidate-kvm-vcpu", 2},
		{"anon_inode:kvm-vcpu-stats:", "candidate-kvm-vcpu-stats", 0},
	} {
		if !strings.HasPrefix(target, kind.prefix) || access != kind.access {
			continue
		}
		suffix := strings.TrimPrefix(target, kind.prefix)
		index, err := strconv.ParseUint(suffix, 10, 32)
		if err == nil && strconv.FormatUint(index, 10) == suffix && index < uint64(d.Image.VCPUs) {
			return kind.role, nil
		}
	}
	return "", ErrPolicy
}
