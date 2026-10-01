//go:build linux

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"github.com/pranavreddyg17/home-node/internal/updates"
	"golang.org/x/sys/unix"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestNativeInspectionDescriptor(t *testing.T) {
	if os.Getenv("HOMENODE_INSPECT_ENTRY_CHILD") == "1" {
		if port := os.Getenv("HOMENODE_INSPECT_DENIED_PORT"); port != "" {
			connection, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", port), time.Second)
			if err == nil {
				connection.Close()
				t.Fatal("inspection reached host listener")
			}
		}
		if hidden := os.Getenv("HOMENODE_INSPECT_HIDDEN_PATH"); hidden != "" {
			if _, err := os.ReadFile(hidden); err == nil {
				t.Fatal("inspection read host temporary marker")
			}
		}
		if os.Getenv("HOMENODE_INSPECT_SERVICE_LIMITS") == "1" {
			verifyInspectionServiceLimits(t)
		}
		duplicate, err := unix.Dup(3)
		if err != nil {
			t.Fatal(err)
		}
		input := os.NewFile(uintptr(duplicate), "expected-package")
		expectedDigest, expectedLength, err := updates.PackageIdentity(context.Background(), input)
		closeErr := input.Close()
		if err != nil || closeErr != nil {
			t.Fatal(err, closeErr)
		}
		var output bytes.Buffer
		if err := run(context.Background(), []string{"--release", "0.1.0~ci", "--operation", os.Getenv("HOMENODE_INSPECT_OPERATION")}, &output); err != nil {
			t.Fatal("worker inspection", err)
		}
		result, err := updates.ValidateInspectionResult(output.Bytes(), updates.InspectionIdentity{OperationID: os.Getenv("HOMENODE_INSPECT_OPERATION"), Release: "0.1.0~ci", PackageSHA256: expectedDigest, PackageLength: expectedLength})
		if err != nil || !result.ContentValid || result.InstallAuthorized {
			t.Fatal("invalid worker result", err)
		}
		return
	}
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_INSPECT_ENTRY_INTEGRATION") != "1" {
		t.Skip("requires explicit disposable Linux root fixture")
	}
	fixture := os.Getenv("HOMENODE_PACKAGE_CONTENT_FIXTURE")
	if fixture == "" {
		t.Fatal("built package fixture missing")
	}
	directory, err := os.MkdirTemp("/tmp", "hn-inspect-entry-")
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
	binary := filepath.Join(directory, "inspect.test")
	copyFile, err := os.OpenFile(binary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0755)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(copyFile, source)
	closeErr := copyFile.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatal(copyErr, closeErr)
	}
	packageBytes, err := os.Open(fixture)
	if err != nil {
		t.Fatal(err)
	}
	defer packageBytes.Close()
	private := filepath.Join(directory, "private")
	if err = os.Mkdir(private, 0700); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(private, "verified.deb")
	staged, err := os.OpenFile(filename, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0400)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr = io.Copy(staged, packageBytes)
	closeErr = staged.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatal(copyErr, closeErr)
	}
	inherited, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer inherited.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "/usr/bin/setpriv", "--bounding-set=-all", "--inh-caps=-all", "--ambient-caps=-all", "--no-new-privs", "--reuid=804", "--regid=804", "--clear-groups", binary, "-test.run=^TestNativeInspectionDescriptor$", "-test.v")
	command.ExtraFiles = []*os.File{inherited}
	command.Env = []string{"HOMENODE_INSPECT_ENTRY_CHILD=1", "HOMENODE_INSPECT_OPERATION=" + rand.Text(), "PATH=/usr/bin:/bin"}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("cross-identity worker failed: %v\n%s", err, output)
	}
}
