package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestRecoveryPublicationDescriptorChecksContentAndRetainsOffset(t *testing.T) {
	if os.Geteuid() == 0 || os.Getegid() == 0 {
		t.Skip("non-root owned-file fixture; root publication ownership requires separate native qualification")
	}
	path := filepath.Join(t.TempDir(), "publication")
	data := []byte("restored management content")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var stat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &stat); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	intent := recoveryPublicationIntent{Version: 1, ConfigurationID: strings.Repeat("a", 32), ConfigurationDigest: strings.Repeat("b", 64), RecoveryIntentSHA256: strings.Repeat("c", 64), FileName: "management.db", ContentSHA256: hex.EncodeToString(hash[:]), Identity: recoveryPublicationIdentity{Device: uint64(stat.Dev), Inode: stat.Ino, Bytes: stat.Size, UID: stat.Uid, GID: stat.Gid}}
	if _, err := file.Seek(3, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	if err := verifyRecoveryPublicationDescriptor(context.Background(), file, intent); err != nil {
		t.Fatal("qualified descriptor refused", err)
	}
	if offset, err := file.Seek(0, io.SeekCurrent); err != nil || offset != 3 {
		t.Fatal("verification changed borrowed offset", offset, err)
	}
	changed := append([]byte(nil), data...)
	changed[0] ^= 1
	if err := os.WriteFile(path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyRecoveryPublicationDescriptor(context.Background(), file, intent); !errors.Is(err, ErrConflict) {
		t.Fatal("same-size content drift admitted", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	if err := verifyRecoveryPublicationDescriptor(context.Background(), file, intent); !errors.Is(err, ErrConflict) {
		t.Fatal("permission drift admitted", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, path+".alias"); err != nil {
		t.Fatal(err)
	}
	if err := verifyRecoveryPublicationDescriptor(context.Background(), file, intent); !errors.Is(err, ErrConflict) {
		t.Fatal("aliased publication descriptor admitted", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := verifyRecoveryPublicationDescriptor(ctx, file, intent); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled verification continued", err)
	}
}
