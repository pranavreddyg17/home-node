package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestRootEndpointCrashRecovery(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-only temporary endpoint fixture")
	}
	for _, network := range []string{"unix", "unixpacket"} {
		t.Run(network, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "service.sock")
			parents, err := lockEndpointParents([]string{path, filepath.Join(dir, "other.sock")})
			if err != nil {
				t.Fatal(err)
			}
			defer parents[dir].Close()
			if len(parents) != 1 {
				t.Fatal("parent lock not shared")
			}
			if other, err := lockEndpointParents([]string{path}); err == nil {
				for _, p := range other {
					p.Close()
				}
				t.Fatal("concurrent supervisor admitted")
			}
			listener, err := net.ListenUnix(network, &net.UnixAddr{Net: network, Name: path})
			if err != nil {
				t.Fatal(err)
			}
			listener.SetUnlinkOnClose(false)
			defer listener.Close()
			if err := os.Chown(path, 0, 1); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0660); err != nil {
				t.Fatal(err)
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := retireStaleEndpoint(parents[dir], path, network, 1); err == nil {
				t.Fatal("live listener removed")
			}
			after, err := os.Lstat(path)
			if err != nil || !os.SameFile(before, after) {
				t.Fatal("live endpoint changed", err)
			}
			if err := listener.Close(); err != nil {
				t.Fatal(err)
			}
			if err := retireStaleEndpoint(parents[dir], path, network, 2); err == nil {
				t.Fatal("foreign group admitted")
			}
			if err := retireStaleEndpoint(parents[dir], path, network, 1); err != nil {
				t.Fatal(err)
			}
			replacement, err := net.ListenUnix(network, &net.UnixAddr{Net: network, Name: path})
			if err != nil {
				t.Fatal("restart could not bind", err)
			}
			replacement.Close()
		})
	}
}

func TestRootEndpointForeignEntryPreserved(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-only temporary endpoint fixture")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "service.sock")
	parents, err := lockEndpointParents([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	defer parents[dir].Close()
	if err := os.WriteFile(path, []byte("retained"), 0660); err != nil {
		t.Fatal(err)
	}
	if err := retireStaleEndpoint(parents[dir], path, "unix", 0); err == nil {
		t.Fatal("regular file removed")
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "retained" {
		t.Fatal("foreign contents changed")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", path); err != nil {
		t.Fatal(err)
	}
	if err := retireStaleEndpoint(parents[dir], path, "unix", 0); err == nil {
		t.Fatal("symlink removed")
	}
	if _, err := os.Readlink(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0777); err != nil {
		t.Fatal(err)
	}
	if err := retireStaleEndpoint(parents[dir], path, "unix", 0); err == nil {
		t.Fatal("parent drift admitted")
	}
}

func TestRootEndpointInterruptedPublication(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-only temporary endpoint fixture")
	}
	for _, gid := range []int{0, 1} {
		t.Run(fmt.Sprint(gid), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "service.sock")
			parents, err := lockEndpointParents([]string{path})
			if err != nil {
				t.Fatal(err)
			}
			defer parents[dir].Close()
			listener, err := net.ListenUnix("unix", &net.UnixAddr{Net: "unix", Name: path})
			if err != nil {
				t.Fatal(err)
			}
			listener.SetUnlinkOnClose(false)
			defer listener.Close()
			if err := os.Chown(path, 0, gid); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0700); err != nil {
				t.Fatal(err)
			}
			if err := retireStaleEndpoint(parents[dir], path, "unix", 1); err == nil {
				t.Fatal("live initializing socket removed")
			}
			listener.Close()
			if err := retireStaleEndpoint(parents[dir], path, "unix", 1); err != nil {
				t.Fatal("interrupted publication not recovered", err)
			}
		})
	}
}
