//go:build linux

package updates

import (
	"context"
	"os"
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err = ValidateDistributionPackage(ctx, file, release); err != nil {
		t.Fatal("built package content rejected", err)
	}
}
