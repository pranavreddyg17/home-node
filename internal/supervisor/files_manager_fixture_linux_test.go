//go:build linux

package supervisor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/catalog"
)

// Development input only: this digest check is not a signed release catalog.
// Copy through a retained source descriptor into an exclusive fixture-owned file.
func stageNativeFilesImage(t *testing.T, ctx context.Context, directory string, group uint32) catalog.Image {
	t.Helper()
	path := os.Getenv("HOMENODE_FILES_IMAGE")
	digest := os.Getenv("HOMENODE_FILES_IMAGE_SHA256")
	size, err := strconv.ParseInt(os.Getenv("HOMENODE_FILES_IMAGE_BYTES"), 10, 64)
	decoded, decodeErr := hex.DecodeString(digest)
	if ctx.Err() != nil || !filepath.IsAbs(path) || filepath.Clean(path) != path || err != nil || size <= 0 || size > 8<<30 || decodeErr != nil || len(decoded) != sha256.Size || hex.EncodeToString(decoded) != digest {
		t.Fatal("unqualified development Files input")
	}
	source, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	before, err := source.Stat()
	if err != nil || !before.Mode().IsRegular() || before.Size() != size || before.Mode().Perm()&0022 != 0 {
		t.Fatal("unqualified development Files source", err)
	}
	destination, err := os.OpenFile(filepath.Join(directory, digest+".raw"), os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer destination.Close()
	hash := sha256.New()
	if n, err := io.CopyN(io.MultiWriter(destination, hash), source, size); err != nil || n != size || hex.EncodeToString(hash.Sum(nil)) != digest {
		t.Fatal("development Files image copy or digest mismatch", err)
	}
	after, err := source.Stat()
	if err != nil || !os.SameFile(before, after) || after.Size() != size || after.Mode() != before.Mode() || !after.ModTime().Equal(before.ModTime()) || ctx.Err() != nil {
		t.Fatal("development Files source changed", err)
	}
	if err := destination.Chown(0, int(group)); err != nil {
		t.Fatal(err)
	}
	if err := destination.Chmod(0440); err != nil {
		t.Fatal(err)
	}
	if err := destination.Sync(); err != nil {
		t.Fatal(err)
	}
	parent, err := os.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := parent.Sync(); err != nil {
		parent.Close()
		t.Fatal(err)
	}
	if err := parent.Close(); err != nil {
		t.Fatal(err)
	}
	return catalog.Image{ID: "files", SHA256: digest, Bytes: size, Protocol: 1, MemoryMiB: 512, VCPUs: 1, DataBytes: 2 * catalog.GiB, Version: "development", License: "Development fixture; release license inventory unqualified"}
}

func stageNativeFilesClient(t *testing.T, base string) string {
	t.Helper()
	directory := filepath.Join(base, "files-client")
	if err := os.Mkdir(directory, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"manager_channel.py", "boot_image.py", "boot_evidence.py", "overlay.py"} {
		source, err := os.OpenFile(filepath.Join("../../packaging/guest", name), os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			t.Fatal(err)
		}
		info, err := source.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 128<<10 {
			source.Close()
			t.Fatal("invalid development client module", err)
		}
		data, err := io.ReadAll(io.LimitReader(source, (128<<10)+1))
		closeErr := source.Close()
		if err != nil || closeErr != nil || int64(len(data)) != info.Size() {
			t.Fatal("development client module changed", err, closeErr)
		}
		file, err := os.OpenFile(filepath.Join(directory, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0644)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = file.Write(data); err == nil {
			err = file.Chmod(0644)
		}
		if err == nil {
			err = file.Sync()
		}
		closeErr = file.Close()
		if err != nil || closeErr != nil {
			t.Fatal("development client staging failed", err, closeErr)
		}
	}
	return filepath.Join(directory, "manager_channel.py")
}

func roundTripNativeFilesChannel(t *testing.T, ctx context.Context, script, channel string, transferGID int, objectID string, attempt int64) {
	t.Helper()
	if !filepath.IsAbs(script) || transferGID < 1 || transferGID > 1<<31-1 {
		t.Fatal("invalid native Files client authority")
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/python3", "-B", script, channel, objectID, strconv.FormatInt(attempt, 10))
	cmd.Dir = filepath.Dir(script)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOMENODE_FILES_MANAGER_INTEGRATION=1"}
	cmd.WaitDelay = time.Second
	// UID 2 is the fixture's transfer UID; primary group grants channel traversal.
	// No supplementary KVM group or root privilege is passed to the client.
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: 2, Gid: uint32(transferGID), Groups: []uint32{}}}
	output, err := cmd.CombinedOutput()
	if err != nil || string(output) != "development Files channel round trip passed\n" {
		t.Fatal("native Files client round trip", err, string(output))
	}
}
