//go:build linux

package updates

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// Generated development evidence must pass the product's package binding gate.
// This does not qualify complete dependency/license/vulnerability evidence.
func TestNativeBuiltPackageSBOM(t *testing.T) {
	filename := os.Getenv("HOMENODE_PACKAGE_CONTENT_FIXTURE")
	if filename == "" || os.Geteuid() == 0 {
		t.Skip("requires explicit development package on disposable unprivileged Linux CI")
	}
	file, err := os.OpenFile(filename, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	hash, _, err := PackageIdentity(ctx, file)
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := os.OpenFile(filename+".sbom.json", os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	info, err := evidence.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 8<<20 {
		evidence.Close()
		t.Fatal("invalid generated SBOM file", err)
	}
	data, readErr := io.ReadAll(io.LimitReader(evidence, (8<<20)+1))
	if err := errors.Join(readErr, evidence.Close()); err != nil {
		t.Fatal(err)
	}
	if len(data) > 8<<20 {
		t.Fatal("oversized generated SBOM")
	}
	if err := ValidateSBOMBinding(data, hash, "0.1.0~ci"); err != nil {
		t.Fatal("generated SBOM refused", err)
	}
	if err := ValidateSBOMBinding(data, hash, "0.2.0~ci"); !errors.Is(err, ErrSBOMBinding) {
		t.Fatal("wrong release admitted", err)
	}
	changed := bytes.Replace(data, []byte(hash), bytes.Repeat([]byte("0"), 64), 1)
	if bytes.Equal(changed, data) {
		t.Fatal("generated package hash not retained")
	}
	if err := ValidateSBOMBinding(changed, hash, "0.1.0~ci"); !errors.Is(err, ErrSBOMBinding) {
		t.Fatal("changed package claim admitted", err)
	}
}
