package supervisor

import (
	"errors"
	"github.com/pranavreddyg17/home-node/internal/state"
	"testing"
)

func TestQEMUDeviceMetadata(t *testing.T) {
	d := Domain{ID: state.Random(), GuestUID: 200000, GuestGID: 108}
	for _, target := range []string{"/dev/null", "/dev/urandom", "/dev/kvm"} {
		t.Run(target, func(t *testing.T) {
			identity := qemuDeviceIdentity{qemuRegularIdentity: qemuRegularIdentity{Device: 7, Inode: 5, Links: 1, Mode: 0020000 | 0666}, Major: 1, Minor: 3}
			access, expected := uint32(2), "null-device"
			if target == "/dev/urandom" {
				identity.Minor = 9
				access = 0
				expected = "random-device"
			}
			if target == "/dev/kvm" {
				identity.GID = d.GuestGID
				identity.Mode = 0020000 | 0660
				identity.Major = 10
				identity.Minor = 232
				expected = "kvm-device"
			}
			if role, err := qualifyQEMUDevice(d, target, identity, identity, access); err != nil || role != expected {
				t.Fatal(role, err)
			}
			for _, mutation := range []string{"owner", "group", "mode", "links", "unlinked", "zero-inode", "major", "minor", "named", "foreign", "access", "invalid-id", "invalid-uid", "invalid-gid"} {
				t.Run(mutation, func(t *testing.T) {
					domain, path, open, named, flags := d, target, identity, identity, access
					switch mutation {
					case "owner":
						open.UID++
						named = open
					case "group":
						open.GID++
						named = open
					case "mode":
						open.Mode ^= 0001
						named = open
					case "links":
						open.Links = 2
						named = open
					case "unlinked":
						open.Links = 0
						named = open
					case "zero-inode":
						open.Inode = 0
						named = open
					case "major":
						open.Major++
						named = open
					case "minor":
						open.Minor++
						named = open
					case "named":
						named.Inode++
					case "foreign":
						path = "/dev/zero"
					case "access":
						flags = 1
					case "invalid-id":
						domain.ID = "../foreign"
					case "invalid-uid":
						domain.GuestUID = 0
					case "invalid-gid":
						domain.GuestGID = 0
					}
					if role, err := qualifyQEMUDevice(domain, path, open, named, flags); role != "" || !errors.Is(err, ErrPolicy) {
						t.Fatal("unqualified device admitted", role, err)
					}
				})
			}
		})
	}
}
