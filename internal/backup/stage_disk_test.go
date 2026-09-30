package backup

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
)

func diskStageFixture(t *testing.T) (*os.Root, *os.File, BackupFile, string) {
	t.Helper()
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	source, err := os.CreateTemp(t.TempDir(), "source-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { source.Close() })
	const size int64 = 16 << 20
	if err = source.Truncate(size); err != nil {
		t.Fatal(err)
	}
	if _, err = source.WriteAt([]byte("leased disk fixture"), 4096); err != nil {
		t.Fatal(err)
	}
	return root, source, BackupFile{Workload: "files", Name: "files.raw", Bytes: size, ImageSHA256: strings.Repeat("a", 64), DataSchema: 1, Protocol: 1}, directory
}

func TestStageDiskRecordsExactBytesAndChecksumWithoutChangingSourceOffset(t *testing.T) {
	root, source, entry, _ := diskStageFixture(t)
	if _, err := source.Seek(100, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	result, err := StageDisk(context.Background(), root, source, entry)
	if err != nil {
		t.Fatal(err)
	}
	file, err := root.Open(result.Name)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	hash := sha256.New()
	n, err := io.Copy(hash, file)
	if err != nil || n != entry.Bytes || result.SHA256 != hex.EncodeToString(hash.Sum(nil)) {
		t.Fatal(result, n, err)
	}
	info, err := file.Stat()
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal(info, err)
	}
	if offset, err := source.Seek(0, io.SeekCurrent); err != nil || offset != 100 {
		t.Fatal("shared source offset changed", offset, err)
	}
}

type stagingCancelContext struct {
	context.Context
	cancel  context.CancelFunc
	path    string
	replace bool
}

func (c *stagingCancelContext) Err() error {
	if info, err := os.Stat(c.path); err == nil && info.Size() > 0 {
		if c.replace {
			if err = os.Rename(c.path, c.path+".partial"); err == nil {
				_ = os.WriteFile(c.path, []byte("retained replacement"), 0600)
			}
			c.replace = false
		}
		c.cancel()
	}
	return c.Context.Err()
}

func TestStageDiskCancellationRemovesOnlyItsOwnPartialOutput(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(map[bool]string{false: "partial", true: "replacement"}[replace], func(t *testing.T) {
			root, source, entry, directory := diskStageFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fault := &stagingCancelContext{Context: ctx, cancel: cancel, path: filepath.Join(directory, entry.Name), replace: replace}
			result, err := StageDisk(fault, root, source, entry)
			if !errors.Is(err, context.Canceled) || result != (BackupFile{}) {
				t.Fatal("partial copy reported success", result, err)
			}
			if replace {
				data, err := os.ReadFile(fault.path)
				if err != nil || string(data) != "retained replacement" {
					t.Fatal("cleanup removed replacement", string(data), err)
				}
			} else if _, err := root.Lstat(entry.Name); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("partial output retained", err)
			}
		})
	}
}

func TestStageDiskRefusesExistingOutputAndInvalidMetadata(t *testing.T) {
	root, source, entry, _ := diskStageFixture(t)
	if err := os.WriteFile(filepath.Join(root.Name(), entry.Name), []byte("prior"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := StageDisk(context.Background(), root, source, entry); !errors.Is(err, os.ErrExist) {
		t.Fatal("existing output overwritten", err)
	}
	data, err := os.ReadFile(filepath.Join(root.Name(), entry.Name))
	if err != nil || string(data) != "prior" {
		t.Fatal(string(data), err)
	}
	for _, modify := range []func(*BackupFile){func(e *BackupFile) { e.Name = "../escape.raw" }, func(e *BackupFile) { e.Bytes-- }, func(e *BackupFile) { e.Workload = "video" }, func(e *BackupFile) { e.SHA256 = strings.Repeat("b", 64) }} {
		invalid := entry
		modify(&invalid)
		if _, err := StageDisk(context.Background(), root, source, invalid); !errors.Is(err, ErrManifest) {
			t.Fatal("invalid metadata admitted", invalid, err)
		}
	}
}
