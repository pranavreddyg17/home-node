//go:build linux

package updates

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDistributionInspectionRefusesUnsafeInputs(t *testing.T) {
	release := ReleaseMetadata{Release: "0.1.0", Platform: "ubuntu-24.04-amd64"}
	if err := ValidateDistributionPackage(context.Background(), nil, release); !errors.Is(err, ErrPackageArchive) {
		t.Fatal(err)
	}
	filename := filepath.Join(t.TempDir(), "package.deb")
	if err := os.WriteFile(filename, debianFixture("control.tar.xz", "data.tar.xz"), 0600); err != nil {
		t.Fatal(err)
	}
	writable, err := os.OpenFile(filename, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer writable.Close()
	if err = ValidateDistributionPackage(context.Background(), writable, release); !errors.Is(err, ErrPackageArchive) {
		t.Fatal("writable descriptor admitted", err)
	}
	file, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if os.Geteuid() == 0 {
		if err = ValidateDistributionPackage(context.Background(), file, release); !errors.Is(err, ErrPackageArchive) {
			t.Fatal("root decoder admitted", err)
		}
		return
	}
	invalid := release
	invalid.Release = "not-a-release"
	if err = ValidateDistributionPackage(context.Background(), file, invalid); !errors.Is(err, ErrPackageControl) {
		t.Fatal("invalid release reached decoder", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = ValidateDistributionPackage(ctx, file, release); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	if err = ValidateDistributionPackage(context.Background(), file, release); !errors.Is(err, ErrPackageArchive) {
		t.Fatal("closed descriptor admitted", err)
	}
}
