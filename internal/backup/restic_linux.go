//go:build linux

package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

var ErrRepository = errors.New("encrypted repository could not be authenticated")

// passwordDescriptor is an anonymous sealed memory file. The password is never
// placed in a shell command, environment variable, disk file, or log message.
func passwordDescriptor(password []byte) (*os.File, error) {
	if len(password) == 0 || len(password) > 8192 || bytes.ContainsAny(password, "\x00\r\n") {
		return nil, ErrRepository
	}
	fd, err := unix.MemfdCreate("homenode-restic-password", unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
	if err != nil {
		return nil, ErrRepository
	}
	file := os.NewFile(uintptr(fd), "restic-password")
	if _, err = file.Write(password); err == nil {
		_, err = file.Seek(0, io.SeekStart)
	}
	if err == nil {
		_, err = unix.FcntlInt(uintptr(fd), unix.F_ADD_SEALS, unix.F_SEAL_WRITE|unix.F_SEAL_GROW|unix.F_SEAL_SHRINK|unix.F_SEAL_SEAL)
	}
	if err != nil {
		_ = file.Close()
		return nil, ErrRepository
	}
	return file, nil
}

type boundedOutput struct {
	data    []byte
	maximum int
}

func repositoryDirectory(parent *os.File, initialize bool) (*os.File, error) {
	if initialize {
		if err := unix.Mkdirat(int(parent.Fd()), "homenode-backup", 0700); err != nil && !errors.Is(err, unix.EEXIST) {
			return nil, ErrRepository
		}
	}
	fd, err := unix.Openat(int(parent.Fd()), "homenode-backup", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, ErrRepository
	}
	repo := os.NewFile(uintptr(fd), "restic-repository")
	var parentStat, repoStat unix.Stat_t
	if unix.Fstat(int(parent.Fd()), &parentStat) != nil || unix.Fstat(fd, &repoStat) != nil || parentStat.Dev != repoStat.Dev {
		_ = repo.Close()
		return nil, ErrRepository
	}
	return repo, nil
}

func (w *boundedOutput) Write(data []byte) (int, error) {
	if len(w.data)+len(data) > w.maximum {
		return 0, ErrRepository
	}
	w.data = append(w.data, data...)
	return len(data), nil
}

// Only the typed operations below reach restic. No user commands, backend URLs,
// shell expansion, inherited RESTIC_* variables or password commands are accepted.
func resticConfig(ctx context.Context, directory *os.File, password []byte, initialize bool) ([]byte, error) {
	if directory == nil {
		return nil, ErrRepository
	}
	info, err := directory.Stat()
	if err != nil || !info.IsDir() {
		return nil, ErrRepository
	}
	secret, err := passwordDescriptor(password)
	if err != nil {
		return nil, err
	}
	defer secret.Close()
	repository, err := repositoryDirectory(directory, initialize)
	if err != nil {
		return nil, err
	}
	defer repository.Close()
	operation := "config"
	if initialize {
		operation = "init"
	}
	return runRestic(ctx, repository, secret, operation)
}

func runRestic(ctx context.Context, repository, secret *os.File, operation string) ([]byte, error) {
	duration := 30 * time.Second
	if operation == "check" {
		duration = 10 * time.Minute
	}
	deadline, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	args := []string{"--repo", "/proc/self/fd/3", "--password-file", "/proc/self/fd/4", "--no-cache", "--json"}
	switch operation {
	case "init":
		args = append(args, "init", "--repository-version", "2")
	case "config":
		args = append(args, "cat", "config")
	case "check":
		args = append(args, "check", "--read-data", "--quiet")
	default:
		return nil, ErrRepository
	}
	output := &boundedOutput{maximum: 32768}
	if err := resticProcess(deadline, args, []*os.File{repository, secret}, output); err != nil {
		return nil, err
	}
	return output.data, nil
}

func resticProcess(ctx context.Context, args []string, descriptors []*os.File, output io.Writer) error {
	cmd := exec.CommandContext(ctx, "/usr/bin/restic", args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "HOME=/nonexistent", "GOMAXPROCS=2", "GOMEMLIMIT=256MiB"}
	cmd.ExtraFiles = descriptors
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Run(); err != nil {
		return ErrRepository
	}
	return nil
}

// Snapshot encrypts a validated recovery set. Maintenance must hold an exclusive
// lease and keep app disks stopped and staging immutable for the entire call.
// This does not itself stop apps or register a drive.
func (r *Repository) Snapshot(ctx context.Context, stage *os.File, manifest Manifest, policy RestorePolicy) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.directory == nil || stage == nil {
		return "", ErrRepository
	}
	root, err := os.OpenRoot("/proc/self/fd/" + strconv.FormatUint(uint64(stage.Fd()), 10))
	if err != nil {
		return "", ErrManifest
	}
	defer root.Close()
	deadline, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	if err = ValidateRecoverySet(deadline, root, manifest, policy); err != nil {
		return "", err
	}
	data, err := json.Marshal(manifest)
	if err != nil || len(data) > MaxManifestBytes {
		return "", ErrManifest
	}
	file, err := root.OpenFile("manifest.json", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		_, writeErr := file.Write(data)
		err = errors.Join(writeErr, file.Sync(), file.Close())
		if err != nil {
			_ = root.Remove("manifest.json")
			return "", ErrManifest
		}
	} else if errors.Is(err, os.ErrExist) {
		info, e := root.Lstat("manifest.json")
		if e != nil || !info.Mode().IsRegular() || info.Size() > MaxManifestBytes {
			return "", ErrManifest
		}
		file, e = root.Open("manifest.json")
		if e != nil {
			return "", ErrManifest
		}
		existing, e := io.ReadAll(io.LimitReader(file, MaxManifestBytes+1))
		_ = file.Close()
		if e != nil || !bytes.Equal(existing, data) {
			return "", ErrManifest
		}
	} else {
		return "", ErrManifest
	}
	if err = stage.Sync(); err != nil {
		return "", ErrManifest
	}
	args := []string{"--repo", "/proc/self/fd/3", "--password-file", "/proc/self/fd/4", "--no-cache", "--json", "backup", "--quiet", "--host", "homenode", "--tag", "homenode-v1", "--", "/proc/self/fd/5/manifest.json"}
	for _, entry := range manifest.Files {
		args = append(args, "/proc/self/fd/5/"+entry.Name)
	}
	output := &boundedOutput{maximum: 32768}
	if err = resticProcess(deadline, args, []*os.File{r.directory, r.secret, stage}, output); err != nil {
		return "", err
	}
	snapshotID, err := parseSnapshotSummary(output.data)
	if err != nil {
		return "", err
	}
	if err = ValidateRecoverySet(deadline, root, manifest, policy); err != nil {
		return "", err
	}
	return snapshotID, nil
}

// VerifyRepository authenticates restic's encrypted config and matches the
// registered repository ID. Call after OpenTarget and before any backup mutation.
func VerifyRepository(ctx context.Context, directory *os.File, target Target, password []byte) error {
	repository, err := openRepository(ctx, directory, target, password)
	if err != nil {
		return err
	}
	return repository.Close()
}

// Repository retains the exact authenticated repository and sealed password.
// Close ends the credential lifetime. It exposes only typed maintenance actions.
type Repository struct {
	mu                sync.Mutex
	directory, secret *os.File
}

// OpenRepository admits the registered mount before authenticating its repository.
func OpenRepository(ctx context.Context, target Target, password []byte) (*Repository, error) {
	directory, err := OpenTarget(target)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	return openRepository(ctx, directory, target, password)
}

func openRepository(ctx context.Context, directory *os.File, target Target, password []byte) (_ *Repository, resultErr error) {
	if err := target.Validate(); err != nil {
		return nil, err
	}
	if directory == nil {
		return nil, ErrRepository
	}
	secret, err := passwordDescriptor(password)
	if err != nil {
		return nil, err
	}
	defer func() {
		if resultErr != nil {
			_ = secret.Close()
		}
	}()
	pinned, err := repositoryDirectory(directory, false)
	if err != nil {
		return nil, err
	}
	defer func() {
		if resultErr != nil {
			_ = pinned.Close()
		}
	}()
	data, err := runRestic(ctx, pinned, secret, "config")
	if err != nil {
		return nil, err
	}
	var config struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err = decoder.Decode(&config); err != nil {
		return nil, ErrRepository
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF || config.ID != target.RepositoryID || (config.Version != 1 && config.Version != 2) {
		return nil, ErrRepository
	}
	return &Repository{directory: pinned, secret: secret}, nil
}

// Check validates every encrypted pack using restic. It does not establish that
// app data can be restored, and it never unlocks or prunes another operation.
func (r *Repository) Check(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.directory == nil {
		return ErrRepository
	}
	_, err := runRestic(ctx, r.directory, r.secret, "check")
	return err
}

func (r *Repository) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.directory == nil {
		return nil
	}
	err := errors.Join(r.directory.Close(), r.secret.Close())
	r.directory, r.secret = nil, nil
	return err
}
