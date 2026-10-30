package supervisor

import (
	"errors"
	"github.com/pranavreddyg17/home-node/internal/state"
	"testing"
)

func TestQEMUAnonymousMetadata(t *testing.T) {
	d := Domain{ID: state.Random(), GuestUID: 200000, GuestGID: 108}
	d.Image.VCPUs = 2
	identity := qemuRegularIdentity{Device: 17, Inode: 1085, Links: 1, Mode: 0600}
	for _, test := range []struct {
		target, role string
		access       uint32
	}{
		{"anon_inode:[eventfd]", "candidate-eventfd", 2},
		{"anon_inode:[signalfd]", "candidate-signalfd", 2},
		{"anon_inode:kvm-vm", "candidate-kvm-vm", 2},
		{"anon_inode:kvm-vcpu:0", "candidate-kvm-vcpu", 2},
		{"anon_inode:kvm-vcpu:1", "candidate-kvm-vcpu", 2},
		{"anon_inode:kvm-vcpu-stats:0", "candidate-kvm-vcpu-stats", 0},
	} {
		t.Run(test.target, func(t *testing.T) {
			if role, err := qualifyQEMUAnonymous(d, test.target, identity, test.access); err != nil || role != test.role {
				t.Fatal(role, err)
			}
			for _, mutation := range []string{"uid", "gid", "mode", "type", "device", "inode", "links", "access", "target", "id", "guest-uid", "guest-gid", "cpus"} {
				t.Run(mutation, func(t *testing.T) {
					domain, target, s, access := d, test.target, identity, test.access
					switch mutation {
					case "uid":
						s.UID++
					case "gid":
						s.GID++
					case "mode":
						s.Mode |= 0001
					case "type":
						s.Mode |= 0100000
					case "device":
						s.Device = 0
					case "inode":
						s.Inode = 0
					case "links":
						s.Links = 0
					case "access":
						access = 1
					case "target":
						target = "anon_inode:[foreign]"
					case "id":
						domain.ID = "../foreign"
					case "guest-uid":
						domain.GuestUID = 0
					case "guest-gid":
						domain.GuestGID = 0
					case "cpus":
						domain.Image.VCPUs = 0
					}
					if role, err := qualifyQEMUAnonymous(domain, target, s, access); role != "" || !errors.Is(err, ErrPolicy) {
						t.Fatal("unqualified anonymous object admitted", role, err)
					}
				})
			}
		})
	}
	for _, target := range []string{"anon_inode:kvm-vcpu:2", "anon_inode:kvm-vcpu:00", "anon_inode:kvm-vcpu:-1", "anon_inode:kvm-vcpu:+1", "anon_inode:kvm-vcpu:4294967296", "anon_inode:kvm-vcpu-stats:2", "anon_inode:kvm-vcpu-stats:", "anon_inode:kvm-vcpu-stats:0 (deleted)"} {
		for _, access := range []uint32{0, 2} {
			if role, err := qualifyQEMUAnonymous(d, target, identity, access); role != "" || !errors.Is(err, ErrPolicy) {
				t.Fatal("unqualified vCPU index admitted", role, err)
			}
		}
	}
}
