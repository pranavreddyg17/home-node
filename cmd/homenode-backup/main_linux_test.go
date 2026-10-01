//go:build linux

package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestNativeBackupEntryActivation(t *testing.T) {
	if os.Getenv("HOMENODE_BACKUP_ENTRY_CHILD") == "1" {
		if os.Geteuid() != 803 || os.Getegid() != 803 {
			t.Fatal("wrong worker identity")
		}
		os.Setenv("LISTEN_PID", strconv.Itoa(os.Getpid()))
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		args := append(validArguments(), "--socket", os.Getenv("HOMENODE_BACKUP_ENTRY_SOCKET"))
		if err := run(ctx, args); err != nil {
			t.Fatal("entry activation failed", err)
		}
		if ctx.Err() == nil || os.Getenv("LISTEN_FDS") != "" || os.Getenv("LISTEN_FDNAMES") != "" {
			t.Fatal("activation shutdown/environment invalid")
		}
		return
	}
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_BACKUP_ENTRY_INTEGRATION") != "1" {
		t.Skip("opt-in disposable root fixture")
	}
	directory, err := os.MkdirTemp("/tmp", "hn-backup-entry-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	if err = os.Chmod(directory, 0755); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.Open(executable)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	binary := filepath.Join(directory, "entry.test")
	target, err := os.OpenFile(binary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0755)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(target, source)
	closeErr := target.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatal(copyErr, closeErr)
	}
	path := filepath.Join(directory, "credential.sock")
	listener, err := net.ListenUnix("unixpacket", &net.UnixAddr{Net: "unixpacket", Name: path})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err = os.Chown(path, 0, 351); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(path, 0660); err != nil {
		t.Fatal(err)
	}
	inherited, err := listener.File()
	if err != nil {
		t.Fatal(err)
	}
	defer inherited.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "-test.run=^TestNativeBackupEntryActivation$", "-test.v")
	command.ExtraFiles = []*os.File{inherited}
	command.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 803, Gid: 803, Groups: []uint32{}}}
	command.Env = []string{"HOMENODE_BACKUP_ENTRY_CHILD=1", "HOMENODE_BACKUP_ENTRY_SOCKET=" + path, "LISTEN_FDS=1", "LISTEN_FDNAMES=homenode-backup-credential"}
	var output bytes.Buffer
	command.Stdout, command.Stderr = &output, &output
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	connection, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unixpacket", path)
	if err != nil {
		cancel()
		command.Wait()
		t.Fatal(err)
	}
	connection.SetDeadline(time.Now().Add(2 * time.Second))
	// Root is deliberately not the configured controller UID: admission must
	// close the connection without attempting repository or staging work.
	data := make([]byte, 1)
	n, readErr := connection.Read(data)
	connection.Close()
	waitErr := command.Wait()
	if n != 0 || readErr == nil || waitErr != nil {
		t.Fatal("foreign peer or joined shutdown failed", n, readErr, waitErr, output.String())
	}
	if timeout, ok := readErr.(net.Error); ok && timeout.Timeout() {
		t.Fatal("foreign peer was not promptly refused")
	}
}
