//go:build linux

package supervisor

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeFreshVolumeFormattingPreservesExistingData(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_VOLUME_INTEGRATION") != "1" {
		t.Skip("opt-in disposable Linux root fixture")
	}
	parent := volumeFixtureDir(t)
	path := filepath.Join(parent, "fresh.raw")
	const size = 64 << 20
	if err := prepareDataVolume(context.Background(), path, size, 4<<30); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var magic [2]byte
	_, err = file.ReadAt(magic[:], 1024+56)
	file.Close()
	if err != nil || magic != [2]byte{0x53, 0xef} {
		t.Fatal("new volume missing ext4 superblock magic")
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() != size || info.Mode().Perm() != 0600 {
		t.Fatal("new volume metadata mismatch")
	}
	existing := filepath.Join(parent, "existing.raw")
	out, err := os.OpenFile(existing, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err := out.Truncate(size); err != nil {
		t.Fatal(err)
	}
	if _, err := out.WriteAt([]byte("existing owner data"), 0); err != nil {
		t.Fatal(err)
	}
	out.Close()
	if err := prepareDataVolume(context.Background(), existing, size, 4<<30); err != nil {
		t.Fatal(err)
	}
	out, err = os.Open(existing)
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, 19)
	_, err = out.ReadAt(data, 0)
	out.Close()
	if err != nil || string(data) != "existing owner data" {
		t.Fatal("existing volume reformatted")
	}
	link := filepath.Join(parent, "symlink.raw")
	if err := os.Symlink(existing, link); err != nil {
		t.Fatal(err)
	}
	if prepareDataVolume(context.Background(), link, size, 4<<30) == nil {
		t.Fatal("symlink volume admitted")
	}
	alias := filepath.Join(parent, "alias.raw")
	if err := os.Link(existing, alias); err != nil {
		t.Fatal(err)
	}
	if prepareDataVolume(context.Background(), existing, size, 4<<30) == nil {
		t.Fatal("hardlink alias volume admitted")
	}
	for _, kind := range []string{"fifo", "directory", "wide", "special", "wrong-size"} {
		t.Run(kind, func(t *testing.T) {
			candidate := filepath.Join(parent, kind+".raw")
			switch kind {
			case "fifo":
				if err := unix.Mkfifo(candidate, 0600); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(candidate, 0700); err != nil {
					t.Fatal(err)
				}
			default:
				f, err := os.OpenFile(candidate, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
				if err != nil {
					t.Fatal(err)
				}
				if err := f.Truncate(size); err != nil {
					t.Fatal(err)
				}
				f.Close()
				switch kind {
				case "wide":
					if err := os.Chmod(candidate, 0644); err != nil {
						t.Fatal(err)
					}
				case "special":
					err := os.Chmod(candidate, 0600|os.ModeSetuid)
					if os.Getenv("HOMENODE_SUPERVISOR_SOURCE_FIXTURE") == "1" {
						if !errors.Is(err, unix.EPERM) {
							t.Fatal("source service allowed special mode creation")
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
				case "wrong-size":
					if err := os.Truncate(candidate, size-1); err != nil {
						t.Fatal(err)
					}
				}
			}
			before, err := os.Lstat(candidate)
			if err != nil {
				t.Fatal(err)
			}
			if prepareDataVolume(context.Background(), candidate, size, 4<<30) == nil {
				t.Fatal("unsafe existing volume admitted")
			}
			after, err := os.Lstat(candidate)
			if err != nil {
				t.Fatal(err)
			}
			if !os.SameFile(before, after) || before.Mode() != after.Mode() || before.Size() != after.Size() {
				t.Fatal("existing volume changed")
			}
		})
	}
	refused := filepath.Join(parent, "reserve.raw")
	if !errors.Is(prepareDataVolume(context.Background(), refused, size, math.MaxInt64-size), ErrCapacity) {
		t.Fatal("unavailable reserve not refused")
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() == "reserve.raw" || strings.HasPrefix(entry.Name(), "reserve.raw.prepare-") {
			t.Fatal("reserve refusal created storage")
		}
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(prepareDataVolume(cancelled, filepath.Join(parent, "cancelled.raw"), size, 4<<30), context.Canceled) {
		t.Fatal("cancelled initialization accepted")
	}
	if _, err := os.Stat(filepath.Join(parent, "cancelled.raw")); !os.IsNotExist(err) {
		t.Fatal("cancelled initialization published data")
	}
}

func TestNativeVolumePublicationIdentity(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_VOLUME_INTEGRATION") != "1" {
		t.Skip("opt-in disposable Linux root fixture")
	}
	const size = 64 << 20
	for _, scenario := range []string{"occupied destination", "swapped staging"} {
		t.Run(scenario, func(t *testing.T) {
			directory := volumeFixtureDir(t)
			root, err := os.OpenRoot(directory)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			file, err := root.OpenFile("stage", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			if err := file.Truncate(size); err != nil {
				t.Fatal(err)
			}
			if scenario == "occupied destination" {
				if err := os.WriteFile(filepath.Join(directory, "data.raw"), []byte("existing owner data"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := publishDataVolume(root, file, "stage", "data.raw", size); !errors.Is(err, unix.EEXIST) {
					t.Fatal("occupied publication did not refuse", err)
				}
				data, err := os.ReadFile(filepath.Join(directory, "data.raw"))
				if err != nil || string(data) != "existing owner data" {
					t.Fatal("occupied destination changed")
				}
			} else {
				if err := root.Rename("stage", "saved"); err != nil {
					t.Fatal(err)
				}
				replacement, err := root.OpenFile("stage", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
				if err != nil {
					t.Fatal(err)
				}
				if err := replacement.Truncate(size); err != nil {
					t.Fatal(err)
				}
				replacement.Close()
				if err := publishDataVolume(root, file, "stage", "data.raw", size); !errors.Is(err, ErrPolicy) {
					t.Fatal("swapped source admitted", err)
				}
				if _, err := root.Stat("data.raw"); !os.IsNotExist(err) {
					t.Fatal("swapped source published")
				}
				if _, err := root.Stat("saved"); err != nil {
					t.Fatal("prepared inode removed")
				}
			}
			if _, err := root.Stat("stage"); err != nil {
				t.Fatal("failed publication removed staging")
			}
		})
	}
}

func volumeFixtureDir(t *testing.T) string {
	t.Helper()
	if os.Getenv("HOMENODE_SUPERVISOR_VOLUME_PARENT") != "/var/lib/homenode/volumes" {
		return t.TempDir()
	}
	directory, err := os.MkdirTemp("/var/lib/homenode/volumes", "fixture-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(directory); err != nil {
			t.Error(err)
		}
	})
	return directory
}
