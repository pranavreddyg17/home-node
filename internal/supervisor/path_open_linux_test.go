//go:build linux

package supervisor

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestReadOnlyComponentOpenRetainsParentAndRefusesLinks(t *testing.T) {
	base := t.TempDir()
	path := filepath.Join(base, "parent")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "value"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	parent, err := openAbsoluteDirectoryNoLinks(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := unix.Close(parent); err != nil {
			t.Error(err)
		}
	})
	if err := os.Symlink(path, filepath.Join(base, "alias")); err != nil {
		t.Fatal(err)
	}
	if fd, err := openAbsoluteDirectoryNoLinks(filepath.Join(base, "alias")); err == nil {
		unix.Close(fd)
		t.Fatal("ancestor symlink admitted")
	}
	if err := os.Symlink("value", filepath.Join(path, "link")); err != nil {
		t.Fatal(err)
	}
	if fd, err := openSameMountReadOnly(parent, "link", false); err == nil {
		unix.Close(fd)
		t.Fatal("child symlink admitted")
	}
	for _, name := range []string{"", ".", "..", "../value", "/value", "value\x00"} {
		if fd, err := openSameMountReadOnly(parent, name, false); !errors.Is(err, ErrPolicy) || fd != -1 {
			if fd >= 0 {
				unix.Close(fd)
			}
			t.Fatal("non-component admitted", name, fd, err)
		}
	}
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "value"), []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := openSameMountReadOnly(parent, "value", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := unix.Close(file); err != nil {
			t.Error(err)
		}
	})
	data := make([]byte, 16)
	n, err := unix.Read(file, data)
	if err != nil || string(data[:n]) != "original" {
		t.Fatal("open redirected by parent replacement", string(data[:n]), err)
	}
	if _, err := unix.Write(file, []byte("modified")); !errors.Is(err, unix.EBADF) {
		t.Fatal("opened descriptor writable", err)
	}
}
