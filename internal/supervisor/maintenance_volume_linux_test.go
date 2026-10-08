//go:build linux

package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/state"
	"golang.org/x/sys/unix"
)

func TestNativeMaintenanceVolume(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_VOLUME_INTEGRATION") != "1" {
		t.Skip("opt-in disposable Linux root fixture")
	}
	directory := volumeFixtureDir(t)
	const size int64 = 16 << 20
	for _, scenario := range []string{"valid", "symlink", "hardlink", "wrong-mode", "wrong-size", "fifo", "parent-sticky", "parent-setgid", "parent-writable"} {
		t.Run(scenario, func(t *testing.T) {
			id := state.Random()
			path := filepath.Join(directory, id+".raw")
			file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
			if err != nil {
				t.Fatal(err)
			}
			if err = file.Truncate(size); err != nil {
				file.Close()
				t.Fatal(err)
			}
			if _, err = file.WriteAt([]byte("pinned"), 0); err != nil {
				file.Close()
				t.Fatal(err)
			}
			file.Close()
			switch scenario {
			case "parent-sticky", "parent-setgid", "parent-writable":
				info, statErr := os.Stat(directory)
				if statErr != nil {
					t.Fatal(statErr)
				}
				original := info.Mode()
				mode := original | os.ModeSticky
				if scenario == "parent-setgid" {
					mode = original | os.ModeSetgid
				}
				if scenario == "parent-writable" {
					mode = original | 0020
				}
				err = os.Chmod(directory, mode)
				defer func() {
					if err := os.Chmod(directory, original); err != nil {
						t.Error(err)
					}
				}()
			case "symlink":
				if err = os.Rename(path, path+".original"); err != nil {
					t.Fatal(err)
				}
				err = os.Symlink(path+".original", path)
			case "hardlink":
				err = os.Link(path, path+".alias")
			case "wrong-mode":
				err = os.Chmod(path, 0644)
			case "wrong-size":
				err = os.Truncate(path, size-1)
			case "fifo":
				if err = os.Remove(path); err != nil {
					t.Fatal(err)
				}
				err = unix.Mkfifo(path, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			pinned, err := openMaintenanceVolume(context.Background(), directory, id, size)
			if scenario != "valid" {
				if err == nil {
					pinned.Close()
					t.Fatal("unsafe inode admitted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer pinned.Close()
			if _, err = pinned.WriteAt([]byte("modified"), 0); err == nil {
				t.Fatal("descriptor writable")
			}
			if err = os.Rename(path, path+".old"); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(path, []byte("replacement"), 0600); err != nil {
				t.Fatal(err)
			}
			data := make([]byte, 6)
			if _, err = pinned.ReadAt(data, 0); err != nil || string(data) != "pinned" {
				t.Fatal("descriptor followed replacement", string(data), err)
			}
		})
	}
	m, _ := newManager(t)
	start := startRequest()
	if _, err := m.Apply(context.Background(), start); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Apply(context.Background(), Request{Version: 1, OperationID: state.Random(), InstanceID: start.InstanceID, Action: "stop", Revision: 2, PolicyGeneration: 1}); err != nil {
		t.Fatal(err)
	}
	instance, err := m.Inspect(context.Background(), start.InstanceID)
	if err != nil {
		t.Fatal(err)
	}
	volume, err := os.OpenFile(filepath.Join(m.Volumes, start.InstanceID+".raw"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = volume.Truncate(instance.DataBytes); err != nil {
		volume.Close()
		t.Fatal(err)
	}
	volume.Close()
	token, err := m.BeginRuntimeMaintenance(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	copyError := errors.New("fixture copy failed")
	var retained *os.File
	err = m.WithMaintenanceDisk(context.Background(), token, start.InstanceID, func(ctx context.Context, file *os.File, got Instance) error {
		retained = file
		if got.ID != start.InstanceID || got.Revision != 2 {
			t.Fatal(got)
		}
		deadline, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
		defer cancel()
		if err := m.EndRuntimeMaintenance(deadline, token); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("barrier released during copy", err)
		}
		return copyError
	})
	if !errors.Is(err, copyError) || retained == nil {
		t.Fatal(err)
	}
	if _, err = retained.Stat(); !errors.Is(err, os.ErrClosed) {
		t.Fatal("descriptor retained after failed copy", err)
	}
	if err = m.EndRuntimeMaintenance(context.Background(), token); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := openMaintenanceVolume(ctx, directory, state.Random(), size); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
