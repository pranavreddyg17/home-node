//go:build linux

package supervisor

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// Generated ordinary files exercise the reader's name/size contract only.
// They do not substitute for authentication of host procfs by the collector.
func TestQEMUProcMetadataReaderBoundsAndNames(t *testing.T) {
	base := t.TempDir()
	if err := os.Mkdir(filepath.Join(base, "fdinfo"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(base, "attr"), 0700); err != nil {
		t.Fatal(err)
	}
	directory, err := os.Open(base)
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	payload := bytes.Repeat([]byte{'m'}, 8192)
	path := filepath.Join(base, "fdinfo", "0")
	if err := os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}
	data, err := readQEMUProcFile(int(directory.Fd()), "fdinfo/0", 8192)
	if err != nil || !bytes.Equal(data, payload) {
		t.Fatal("bounded metadata read", len(data), err)
	}
	if err := os.WriteFile(path, append(payload, 'x'), 0600); err != nil {
		t.Fatal(err)
	}
	if data, err := readQEMUProcFile(int(directory.Fd()), "fdinfo/0", 8192); data != nil || !errors.Is(err, ErrPolicy) {
		t.Fatal("oversized metadata accepted", len(data), err)
	}
	if err := os.WriteFile(filepath.Join(base, "attr", "current"), []byte("generated (enforce)\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := readQEMUProcFile(int(directory.Fd()), "attr/current", 4096); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", "mem", "cmdline", "/attr/current", "attr/../attr/current", "attr/current/", "fdinfo/", "fdinfo/00", "fdinfo/-1", "fdinfo/+1", "fdinfo/1048577", "fdinfo/4294967296", "fdinfo/../mem", "fdinfo/0/../0", "fdinfo/0\x00", "fdinfo/0\n"} {
		if data, err := readQEMUProcFile(int(directory.Fd()), name, 4096); data != nil || !errors.Is(err, ErrPolicy) {
			t.Fatal("unqualified metadata name accepted", name, len(data), err)
		}
	}
	for _, limit := range []int64{-1, 0, 8193, 1 << 62} {
		if data, err := readQEMUProcFile(int(directory.Fd()), "fdinfo/0", limit); data != nil || !errors.Is(err, ErrPolicy) {
			t.Fatal("unqualified metadata bound accepted", limit, len(data), err)
		}
	}
	if data, err := readQEMUProcFile(int(directory.Fd()), "attr/current", 4097); data != nil || !errors.Is(err, ErrPolicy) {
		t.Fatal("label bound exceeded", len(data), err)
	}
	if err := os.Symlink("0", filepath.Join(base, "fdinfo", "1")); err != nil {
		t.Fatal(err)
	}
	if data, err := readQEMUProcFile(int(directory.Fd()), "fdinfo/1", 8192); err == nil || data != nil {
		t.Fatal("metadata symlink followed", len(data), err)
	}
}
