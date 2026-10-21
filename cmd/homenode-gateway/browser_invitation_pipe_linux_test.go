//go:build linux

package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestNativeBrowserInvitationPipeBoundary(t *testing.T) {
	secret := strings.Repeat("x", 32)
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	descriptor, err := unix.Dup(int(writer.Fd()))
	writer.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeNativeBrowserInvitation(strconv.Itoa(descriptor), secret); err != nil {
		t.Fatal(err)
	}
	var value struct{ Invitation string }
	if err := json.NewDecoder(io.LimitReader(reader, 4096)).Decode(&value); err != nil || value.Invitation != secret {
		t.Fatal("private pipe did not receive invitation")
	}
	if remaining, err := io.ReadAll(reader); err != nil || len(remaining) != 0 {
		t.Fatal("handoff did not close its pipe")
	}
	file, err := os.Create(filepath.Join(t.TempDir(), "untouched"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := writeNativeBrowserInvitation(strconv.Itoa(int(file.Fd())), secret); err == nil {
		t.Fatal("regular file accepted as enrollment handoff")
	}
	if info, err := file.Stat(); err != nil || info.Size() != 0 {
		t.Fatal("refused file descriptor was closed or written")
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if err := writeNativeBrowserInvitation(strconv.Itoa(int(r.Fd())), secret); err == nil {
		t.Fatal("read pipe accepted as write handoff")
	}
	for _, fd := range []string{"", "0", "1", "2", "-1", "03", "4097"} {
		if err := writeNativeBrowserInvitation(fd, secret); err == nil {
			t.Fatal("invalid descriptor accepted")
		}
	}
	for _, code := range []string{"", strings.Repeat("x", 257)} {
		if err := writeNativeBrowserInvitation(strconv.Itoa(int(w.Fd())), code); err == nil {
			t.Fatal("invalid invitation bounds accepted")
		}
	}
}
