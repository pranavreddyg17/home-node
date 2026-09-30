//go:build linux

package supervisor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
	"golang.org/x/sys/unix"
)

func TestNativePreparedVolumeCleanup(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_VOLUME_INTEGRATION") != "1" {
		t.Skip("opt-in disposable Linux root fixture")
	}
	const size = 64 << 20
	t.Run("retry abandoned preparation", func(t *testing.T) {
		parent := volumeFixtureDir(t)
		id := state.Random()
		path := filepath.Join(parent, id+".raw")
		stage := path + ".prepare-" + strings.Repeat("a", 32)
		file, err := os.OpenFile(stage, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Truncate(size); err != nil {
			t.Fatal(err)
		}
		file.Close()
		if err := purgeVolumePreparation(context.Background(), parent, id, size); err != nil {
			t.Fatal(err)
		}
		if err := prepareDataVolume(context.Background(), path, size, 4<<30); err != nil {
			t.Fatal(err)
		}
		file, err = os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		var magic [2]byte
		_, err = file.ReadAt(magic[:], 1024+56)
		file.Close()
		if err != nil || magic != [2]byte{0x53, 0xef} {
			t.Fatal("retry did not publish formatted volume")
		}
	})
	t.Run("only scoped staging", func(t *testing.T) {
		parent := volumeFixtureDir(t)
		id := state.Random()
		selected := filepath.Join(parent, id+".raw.prepare-"+strings.Repeat("a", 32))
		other := filepath.Join(parent, state.Random()+".raw.prepare-"+strings.Repeat("b", 32))
		published := filepath.Join(parent, id+".raw")
		for _, path := range []string{selected, other, published} {
			if err := os.WriteFile(path, nil, 0600); err != nil {
				t.Fatal(err)
			}
		}
		if err := purgeVolumePreparation(context.Background(), parent, id, size); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(selected); !os.IsNotExist(err) {
			t.Fatal("owned staging retained")
		}
		for _, path := range []string{other, published} {
			if _, err := os.Stat(path); err != nil {
				t.Fatal("unselected data removed")
			}
		}
	})
	for _, kind := range []string{"symlink", "fifo", "directory", "wide", "hardlink", "malformed", "wrong-size"} {
		t.Run(kind, func(t *testing.T) {
			parent := volumeFixtureDir(t)
			id := state.Random()
			prefix := id + ".raw.prepare-"
			good := filepath.Join(parent, prefix+strings.Repeat("a", 32))
			if err := os.WriteFile(good, nil, 0600); err != nil {
				t.Fatal(err)
			}
			bad := filepath.Join(parent, prefix+strings.Repeat("b", 32))
			switch kind {
			case "symlink":
				if err := os.Symlink(good, bad); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := unix.Mkfifo(bad, 0600); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(bad, 0700); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(good, bad); err != nil {
					t.Fatal(err)
				}
			default:
				if kind == "malformed" {
					bad = filepath.Join(parent, prefix+"not-a-nonce")
				}
				if err := os.WriteFile(bad, nil, 0600); err != nil {
					t.Fatal(err)
				}
				if kind == "wide" {
					if err := os.Chmod(bad, 0644); err != nil {
						t.Fatal(err)
					}
				}
				if kind == "wrong-size" {
					if err := os.Truncate(bad, 1); err != nil {
						t.Fatal(err)
					}
				}
			}
			if purgeVolumePreparation(context.Background(), parent, id, size) == nil {
				t.Fatal("unsafe staging admitted")
			}
			for _, path := range []string{good, bad} {
				if _, err := os.Lstat(path); err != nil {
					t.Fatal("complete-set refusal removed data")
				}
			}
		})
	}
}
