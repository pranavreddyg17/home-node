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
