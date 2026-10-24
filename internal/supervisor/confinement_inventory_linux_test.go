//go:build linux

package supervisor

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

type nativePolicyOutput struct {
	bytes.Buffer
	overflow bool
}

func (b *nativePolicyOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := (2 << 20) - b.Len()
	if len(p) > remaining {
		b.overflow = true
		p = p[:remaining]
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}

// Source inventory, not kernel-policy equivalence or write-confinement proof.
// Preprocessing expands includes without loading or replacing AppArmor policy.
func nativeConfinementInventory(t *testing.T, ctx context.Context, d Domain) {
	t.Helper()
	if os.Getenv("HOMENODE_CONFINEMENT_INVENTORY") != "1" {
		return
	}
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_KVM_DOMAIN_INTEGRATION") != "1" {
		t.Fatal("disposable native inventory opt-in required")
	}
	read := func(path string, limit int64) []byte {
		t.Helper()
		file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, limit+1))
		if err != nil || int64(len(data)) > limit {
			t.Fatal("inventory source exceeds bound", err)
		}
		return data
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(read(filepath.Join("/run/libvirt/qemu", d.Name()+".pid"), 64))))
	if err != nil || pid < 1 {
		t.Fatal("inventory process")
	}
	label := strings.TrimSpace(string(read(filepath.Join("/proc", strconv.Itoa(pid), "attr/current"), 4096)))
	valid, _ := regexp.MatchString(`^libvirt-[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12} \(enforce\)$`, label)
	if !valid {
		t.Fatal("unqualified live AppArmor label")
	}
	name := strings.TrimSuffix(label, " (enforce)")
	path := filepath.Join("/etc/apparmor.d/libvirt", name)
	profile, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer profile.Close()
	info, err := profile.Stat()
	if err != nil {
		t.Fatal(err)
	}
	metadata, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || metadata.Uid != 0 || info.Mode().Perm()&0022 != 0 || info.Size() < 1 || info.Size() > 128<<10 {
		t.Fatal("unqualified generated policy source")
	}
	source, err := io.ReadAll(io.LimitReader(profile, (128<<10)+1))
	if err != nil || len(source) > 128<<10 {
		t.Fatal("generated policy bound", err)
	}
	if _, err := profile.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	parser, err := filepath.EvalSymlinks("/usr/sbin/apparmor_parser")
	if err != nil {
		t.Fatal(err)
	}
	parserInfo, err := os.Lstat(parser)
	if err != nil {
		t.Fatal(err)
	}
	parserMetadata, ok := parserInfo.Sys().(*syscall.Stat_t)
	if !ok || !parserInfo.Mode().IsRegular() || parserMetadata.Uid != 0 || parserInfo.Mode().Perm()&0022 != 0 {
		t.Fatal("unqualified policy parser")
	}
	scope, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(scope, parser, "--preprocess", "/proc/self/fd/3")
	cmd.ExtraFiles = []*os.File{profile}
	cmd.Env = []string{"LANG=C", "LC_ALL=C", "PATH=/usr/sbin:/usr/bin:/sbin:/bin"}
	var output nativePolicyOutput
	var diagnostic boundedOutput
	cmd.Stdout = &output
	cmd.Stderr = &diagnostic
	if err := cmd.Run(); err != nil || output.overflow || diagnostic.tooLarge || output.Len() == 0 {
		t.Fatal("bounded policy preprocessing failed", err)
	}
	if !bytes.Equal(read(path, 128<<10), source) {
		t.Fatal("generated policy changed during inventory")
	}
	if strings.TrimSpace(string(read(filepath.Join("/proc", strconv.Itoa(pid), "attr/current"), 4096))) != label {
		t.Fatal("process label changed during inventory")
	}
	const directory = "/usr/lib/homenode-fixtures/confinement"
	if err := os.Mkdir(directory, 0755); err != nil && !errors.Is(err, os.ErrExist) {
		t.Fatal(err)
	}
	dirInfo, err := os.Lstat(directory)
	if err != nil {
		t.Fatal(err)
	}
	dirMetadata, ok := dirInfo.Sys().(*syscall.Stat_t)
	if !ok || !dirInfo.IsDir() || dirMetadata.Uid != 0 || dirInfo.Mode().Perm()&0022 != 0 {
		t.Fatal("unqualified inventory destination")
	}
	digest := sha256.Sum256(output.Bytes())
	hash := hex.EncodeToString(digest[:])
	destination := filepath.Join(directory, hash+".apparmor")
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0644)
	if errors.Is(err, os.ErrExist) {
		if !bytes.Equal(read(destination, 2<<20), output.Bytes()) {
			t.Fatal("inventory collision")
		}
	} else {
		if err != nil {
			t.Fatal(err)
		}
		_, writeErr := file.Write(output.Bytes())
		syncErr := file.Sync()
		closeErr := file.Close()
		if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
			t.Fatal(err)
		}
	}
	sourceDigest := sha256.Sum256(source)
	t.Log("native AppArmor source inventory", name, "sourceSHA256", hex.EncodeToString(sourceDigest[:]), "expandedSHA256", hash, "bytes", output.Len())
	t.Log("native AppArmor source inventory retained; effective kernel write permissions and safe UID reuse remain unqualified")
}
