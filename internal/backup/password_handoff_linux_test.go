//go:build linux

package backup

import (
	"bytes"
	"context"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func TestSealedRepositoryPasswordHandoffPreservesOffset(t *testing.T) {
	secret := []byte("fixture-secret-not-for-production")
	file, err := CreateRepositoryPassword(secret)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err = file.Seek(4, 0); err != nil {
		t.Fatal(err)
	}
	received, err := ReadRepositoryPassword(context.Background(), file)
	defer clear(received)
	if err != nil || !bytes.Equal(received, secret) {
		t.Fatal("sealed credential handoff refused", err)
	}
	position, err := file.Seek(0, 1)
	if err != nil || position != 4 {
		t.Fatal("handoff changed shared offset", err)
	}
	if _, err = file.WriteAt([]byte("x"), 0); err == nil {
		t.Fatal("handoff credential remained mutable")
	}
}
func TestRepositoryPasswordHandoffRefusesMutableAndDiskDescriptors(t *testing.T) {
	disk, err := os.CreateTemp(t.TempDir(), "password")
	if err != nil {
		t.Fatal(err)
	}
	defer disk.Close()
	if _, err = disk.WriteString("fixture-secret"); err != nil {
		t.Fatal(err)
	}
	fd, err := unix.MemfdCreate("unsealed-fixture", unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
	if err != nil {
		t.Fatal(err)
	}
	mutable := os.NewFile(uintptr(fd), "unsealed")
	defer mutable.Close()
	if _, err = mutable.WriteString("fixture-secret"); err != nil {
		t.Fatal(err)
	}
	pipe, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pipe.Close()
	defer writer.Close()
	for _, file := range []*os.File{disk, mutable, pipe, nil} {
		if value, err := ReadRepositoryPassword(context.Background(), file); err == nil || value != nil {
			t.Fatal("unsafe credential descriptor admitted")
		}
	}
}

func TestRepositoryPasswordHandoffRejectsInvalidSealedPayload(t *testing.T) {
	fd, err := unix.MemfdCreate("invalid-secret-fixture", unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
	if err != nil {
		t.Fatal(err)
	}
	file := os.NewFile(uintptr(fd), "invalid-secret")
	defer file.Close()
	if _, err = file.Write([]byte("invalid\x00secret")); err != nil {
		t.Fatal(err)
	}
	if _, err = unix.FcntlInt(file.Fd(), unix.F_ADD_SEALS, unix.F_SEAL_WRITE|unix.F_SEAL_GROW|unix.F_SEAL_SHRINK|unix.F_SEAL_SEAL); err != nil {
		t.Fatal(err)
	}
	if value, err := ReadRepositoryPassword(context.Background(), file); err == nil || value != nil {
		t.Fatal("invalid sealed payload admitted")
	}
}
