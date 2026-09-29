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
	cmd := exec.CommandContext(deadline, "/usr/bin/restic", args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "HOME=/nonexistent"}
	cmd.ExtraFiles = []*os.File{repository, secret}
	output := &boundedOutput{maximum: 32768}
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Run(); err != nil {
		return nil, ErrRepository
	}
	return output.data, nil
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
