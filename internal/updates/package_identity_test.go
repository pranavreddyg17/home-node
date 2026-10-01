package updates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPackageIdentityPreservesDescriptorAndRejectsInvalidInput(t *testing.T) {
	data := []byte("package identity fixture")
	filename := filepath.Join(t.TempDir(), "package.deb")
	if err := os.WriteFile(filename, data, 0400); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err = file.Seek(5, 0); err != nil {
		t.Fatal(err)
	}
	digest, length, err := PackageIdentity(context.Background(), file)
	expected := sha256.Sum256(data)
	if err != nil || digest != hex.EncodeToString(expected[:]) || length != int64(len(data)) {
		t.Fatal(digest, length, err)
	}
	position, err := file.Seek(0, 1)
	if err != nil || position != 5 {
		t.Fatal("descriptor offset changed", position, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err = PackageIdentity(ctx, file); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, _, err = PackageIdentity(context.Background(), nil); err == nil {
		t.Fatal("missing descriptor accepted")
	}
}
