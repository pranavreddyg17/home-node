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

func TestQEMURegularReadMetadata(t *testing.T) {
	d := Domain{ID: state.Random(), GuestUID: 200000, GuestGID: 108, SystemPath: "/var/lib/homenode/images/system.raw", DataPath: "/var/lib/homenode/volumes/data.raw"}
	image := qemuRegularIdentity{Device: 10, Inode: 100, Links: 1, GID: d.GuestGID, Mode: 0100000 | 0440}
	if role, err := qualifyQEMURegularRead(d, d.SystemPath, image, image, 0); err != nil || role != "readonly-system-image" {
		t.Fatal(role, err)
	}
	for _, mutation := range []string{"owner", "group", "writable", "links", "deleted", "inode", "named", "foreign", "alias", "readwrite", "writeonly", "invalid-id", "invalid-uid", "invalid-gid", "relative", "unclean", "symlink"} {
		t.Run(mutation, func(t *testing.T) {
			domain, target, open, named, access := d, d.SystemPath, image, image, uint32(0)
			switch mutation {
			case "owner":
				open.UID = d.GuestUID
				named = open
			case "group":
				open.GID++
				named = open
			case "writable":
				open.Mode |= 0200
				named = open
			case "links":
				open.Links = 2
				named = open
			case "deleted":
				open.Links = 0
				named = open
			case "inode":
				open.Inode = 0
				named = open
			case "named":
				named.Inode++
			case "foreign":
				target = "/var/lib/homenode/images/other.raw"
			case "alias":
				domain.DataPath = domain.SystemPath
			case "readwrite":
				access = 2
			case "writeonly":
				access = 1
			case "invalid-id":
				domain.ID = "../foreign"
			case "invalid-uid":
				domain.GuestUID = 0
			case "invalid-gid":
				domain.GuestGID = 0
			case "relative":
				target = "system.raw"
				domain.SystemPath = target
			case "unclean":
				target = "/var/lib/homenode/images/../system.raw"
				domain.SystemPath = target
			case "symlink":
				open.Mode = 0120000 | 0440
				named = open
			}
			if role, err := qualifyQEMURegularRead(domain, target, open, named, access); role != "" || !errors.Is(err, ErrPolicy) {
				t.Fatal("unqualified image descriptor admitted", role, err)
			}
		})
	}
}
