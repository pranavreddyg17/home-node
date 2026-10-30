//go:build linux

package supervisor

import (
	"bytes"
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func TestQEMUSocketObservationPreservesQueuedDataAndFlags(t *testing.T) {
	sockets, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_CLOEXEC|unix.SOCK_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(sockets[0])
	defer unix.Close(sockets[1])
	payload := []byte("generated queued metadata-observation control")
	if n, err := unix.Write(sockets[1], payload); err != nil || n != len(payload) {
		t.Fatal(n, err)
	}
	flags, err := unix.FcntlInt(uintptr(sockets[0]), unix.F_GETFL, 0)
	if err != nil {
		t.Fatal(err)
	}
	var identity unix.Stat_t
	if err := unix.Fstat(sockets[0], &identity); err != nil {
		t.Fatal(err)
	}
	metadata, err := readQEMUSocketMetadata(sockets[0], identity)
	if err != nil || metadata.Domain != unix.AF_UNIX || metadata.Type != unix.SOCK_STREAM || metadata.Listening != 0 || metadata.PeerCredentials == nil || metadata.PeerCredentials.Pid != int32(os.Getpid()) {
		t.Fatal(metadata, err)
	}
	after, err := unix.FcntlInt(uintptr(sockets[0]), unix.F_GETFL, 0)
	if err != nil || after != flags {
		t.Fatal("shared flags changed", after, flags, err)
	}
	data := make([]byte, len(payload))
	if n, err := unix.Read(sockets[0], data); err != nil || n != len(payload) || !bytes.Equal(data, payload) {
		t.Fatal("queued data changed", n, err)
	}
	identity.Ino++
	if _, err := readQEMUSocketMetadata(sockets[0], identity); !errors.Is(err, ErrPolicy) {
		t.Fatal("foreign inode admitted", err)
	}
}

func TestQEMUSocketObservationRequiresSocketIdentity(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "generated-regular")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var identity unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &identity); err != nil {
		t.Fatal(err)
	}
	if _, err := readQEMUSocketMetadata(int(f.Fd()), identity); !errors.Is(err, ErrPolicy) {
		t.Fatal("regular descriptor accepted", err)
	}
}
