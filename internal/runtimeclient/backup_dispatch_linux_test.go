//go:build linux

package runtimeclient

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeBackupCredentialSocketAdmission(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_BACKUP_DISPATCH_INTEGRATION") != "1" {
		t.Skip("explicit disposable root fixture")
	}
	directory, err := os.MkdirTemp("/tmp", "hn-dispatch-admission-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	if err = os.Chmod(directory, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "credential.sock")
	listener, err := net.ListenUnix("unixpacket", &net.UnixAddr{Net: "unixpacket", Name: path})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	reset := func(t *testing.T) {
		t.Helper()
		if err := os.Chown(path, 0, 351); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0660); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(directory, 0755); err != nil {
			t.Fatal(err)
		}
	}
	reset(t)
	if err = validateBackupCredentialSocket(path, 351); err != nil {
		t.Fatal("protected controller socket refused", err)
	}
	for _, scenario := range []string{"public-socket", "foreign-owner", "foreign-group", "writable-parent"} {
		t.Run(scenario, func(t *testing.T) {
			reset(t)
			switch scenario {
			case "public-socket":
				err = os.Chmod(path, 0664)
			case "foreign-owner":
				err = os.Chown(path, 803, 351)
			case "foreign-group":
				err = os.Chown(path, 0, 352)
			case "writable-parent":
				err = os.Chmod(directory, 0775)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = validateBackupCredentialSocket(path, 351); err == nil {
				t.Fatal("unsafe socket admitted", scenario)
			}
		})
	}
	reset(t)
	alias := filepath.Join(directory, "alias.sock")
	if err = os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	if err = validateBackupCredentialSocket(alias, 351); err == nil {
		t.Fatal("socket alias admitted")
	}
}
