//go:build linux

package updates

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// This opt-in build qualification uses distribution tools on the disposable CI
// host. It is not the production inspection sandbox or an install authority.
func TestNativeBuiltPackageContent(t *testing.T) {
	filename := os.Getenv("HOMENODE_PACKAGE_CONTENT_FIXTURE")
	if filename == "" || os.Geteuid() == 0 {
		t.Skip("requires explicit development package on disposable unprivileged Linux CI")
	}
	file, err := os.OpenFile(filename, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err = InspectDebianArchive(file); err != nil {
		t.Fatal("built package structure", err)
	}
	release := ReleaseMetadata{Release: "0.1.0~ci", Platform: "ubuntu-24.04-amd64"}
	for _, role := range []string{"control", "data"} {
		t.Run(role, func(t *testing.T) {
			if _, err := file.Seek(0, 0); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			flag := "--ctrl-tarfile"
			if role == "data" {
				flag = "--fsys-tarfile"
			}
			command := exec.CommandContext(ctx, "/usr/bin/dpkg-deb", flag, "/proc/self/fd/3")
			command.ExtraFiles = []*os.File{file}
			command.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
			command.Stderr = io.Discard
			pipe, err := command.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err = command.Start(); err != nil {
				t.Fatal(err)
			}
			if role == "control" {
				err = ValidateControlArchive(pipe, release)
			} else {
				err = ValidatePayloadArchive(ctx, pipe)
			}
			if err != nil {
				cancel()
			}
			closeErr := pipe.Close()
			waitErr := command.Wait()
			if err = errors.Join(err, closeErr, waitErr); err != nil {
				t.Fatal("built package content rejected", err)
			}
		})
	}
}
