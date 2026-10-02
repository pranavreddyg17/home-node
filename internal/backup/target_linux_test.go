//go:build linux

package backup

import (
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"testing"
)

func TestMissingRegisteredDriveNeverOpensHostDirectory(t *testing.T) {
	target := registeredTarget()
	target.MountPath = t.TempDir()
	target.UUID = "homenode-nonexistent-test-drive"
	if directory, err := OpenTarget(target); err == nil {
		directory.Close()
		t.Fatal("missing drive fell back to host directory")
	}
}

func TestTargetDirectoryOpenRejectsReplacementSymlink(t *testing.T) {
	directory := t.TempDir()
	link := filepath.Join(t.TempDir(), "replaced-target")
	if err := os.Symlink(directory, link); err != nil {
		t.Fatal(err)
	}
	if file, err := openTargetDirectory(link); err == nil || file != nil {
		if file != nil {
			file.Close()
		}
		t.Fatal("replacement symlink followed")
	}
	file, err := openTargetDirectory(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	flags, err := unix.FcntlInt(file.Fd(), unix.F_GETFD, 0)
	if err != nil || flags&unix.FD_CLOEXEC == 0 {
		t.Fatal("target descriptor inheritable", err)
	}
}

func TestTargetDescriptorMountIdentity(t *testing.T) {
	file, err := openTargetDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var info unix.Statx_t
	if err := unix.Statx(int(file.Fd()), "", unix.AT_EMPTY_PATH, unix.STATX_MNT_ID, &info); err != nil {
		t.Fatal(err)
	}
	if err := requireTargetMountID(file, info.Mnt_id); err != nil {
		t.Fatal(err)
	}
	for _, wrong := range []uint64{0, info.Mnt_id + 1} {
		if err := requireTargetMountID(file, wrong); err == nil {
			t.Fatal("wrong mount admitted")
		}
	}
}

func TestTargetDirectoryOpenRejectsSymlinkParentAndMagicLink(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "real")
	if err := os.MkdirAll(filepath.Join(real, "target"), 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "parent")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(link, "target"), "/proc/self/root" + filepath.Join(real, "target")} {
		file, err := openTargetDirectory(path)
		if err == nil || file != nil {
			if file != nil {
				file.Close()
			}
			t.Fatal("symlink path resolved", path)
		}
	}
}

func TestObservedTargetRejectsSameFilesystemDirectoryReplacement(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	expected, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := openObservedTargetDirectory(target, expected)
	if err != nil {
		t.Fatal(err)
	}
	pinned.Close()
	if err := os.Rename(target, filepath.Join(base, "old")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if file, err := openObservedTargetDirectory(target, expected); err == nil || file != nil {
		if file != nil {
			file.Close()
		}
		t.Fatal("same filesystem replacement admitted")
	}
}
