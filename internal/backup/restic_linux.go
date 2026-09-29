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
	repository, err := repositoryDirectory(directory, initialize)
	if err != nil {
		return nil, err
	}
	defer repository.Close()
	secret, err := passwordDescriptor(password)
	if err != nil {
		return nil, err
	}
	defer secret.Close()
	deadline, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	args := []string{"--repo", "/proc/self/fd/3", "--password-file", "/proc/self/fd/4", "--no-cache", "--json"}
	if initialize {
		args = append(args, "init", "--repository-version", "2")
	} else {
		args = append(args, "cat", "config")
	}
	cmd := exec.CommandContext(deadline, "/usr/bin/restic", args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "HOME=/nonexistent"}
	cmd.ExtraFiles = []*os.File{repository, secret}
	output := &boundedOutput{maximum: 32768}
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 2 * time.Second
	if err = cmd.Run(); err != nil {
		return nil, ErrRepository
	}
	return output.data, nil
}

// VerifyRepository authenticates restic's encrypted config and matches the
// registered repository ID. Call after OpenTarget and before any backup mutation.
func VerifyRepository(ctx context.Context, directory *os.File, target Target, password []byte) error {
	if err := target.Validate(); err != nil {
		return err
	}
	data, err := resticConfig(ctx, directory, password, false)
	if err != nil {
		return err
	}
	var config struct {
		ID      string `json:"id"`
		Version int    `json:"version"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err = decoder.Decode(&config); err != nil {
		return ErrRepository
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF || config.ID != target.RepositoryID || (config.Version != 1 && config.Version != 2) {
		return ErrRepository
	}
	return nil
}
