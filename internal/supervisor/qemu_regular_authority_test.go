package supervisor

import (
	"errors"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestQEMURegularWriteMetadata(t *testing.T) {
	d := Domain{ID: state.Random(), GuestUID: 200000, GuestGID: 108, DataPath: "/var/lib/homenode/volumes/video.raw"}
	data := qemuRegularIdentity{Device: 10, Inode: 100, Links: 1, UID: d.GuestUID, GID: d.GuestGID, Mode: 0100000 | 0600}
	pid := qemuRegularIdentity{Device: 20, Inode: 200, Links: 1, Mode: 0100000 | 0644}
	for _, kind := range []string{"data", "pid"} {
		t.Run(kind, func(t *testing.T) {
			path, identity, access, expected := d.DataPath, data, uint32(2), "managed-data"
			if kind == "pid" {
				path = "/run/libvirt/qemu/" + d.Name() + ".pid"
				identity = pid
				access = 1
				expected = "candidate-domain-pidfile"
			}
			role, err := qualifyQEMURegularWrite(d, path, identity, identity, access)
			if err != nil || role != expected {
				t.Fatal(role, err)
			}
			for _, mutation := range []string{"owner", "group", "mode", "links", "deleted", "named-inode", "named-device", "named-owner", "readonly", "invalid-access", "foreign", "log", "symlink"} {
				t.Run(mutation, func(t *testing.T) {
					changed, named, flags, target := identity, identity, access, path
					switch mutation {
					case "owner":
						changed.UID++
						named = changed
					case "group":
						changed.GID++
						named = changed
					case "mode":
						changed.Mode |= 0002
						named = changed
					case "links":
						changed.Links = 2
						named = changed
					case "deleted":
						changed.Links = 0
						named = changed
					case "named-inode":
						named.Inode++
					case "named-device":
						named.Device++
					case "named-owner":
						named.UID++
					case "readonly":
						flags = 0
					case "invalid-access":
						flags = 3
					case "foreign":
						target = "/run/libvirt/qemu/homenode-foreign.pid"
					case "log":
						target = "/var/log/libvirt/qemu/" + d.Name() + ".log"
					case "symlink":
						changed.Mode = 0120000 | 0644
						named = changed
					}
					if role, err := qualifyQEMURegularWrite(d, target, changed, named, flags); !errors.Is(err, ErrPolicy) || role != "" {
						t.Fatal("unqualified regular authority admitted", role, err)
					}
				})
			}
		})
	}
}
