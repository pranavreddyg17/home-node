package install

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

type Engine struct {
	host, journalRoot *os.Root
	lock              *os.File
	owner             int
	mu                sync.Mutex
	checkpoint        func(stage, name string) error
}

// Open requires an existing private journal directory and a protected host
// root. The production orchestrator must call this as root with hostRoot="/".
// Tests use a private temporary root; no system configuration is changed.
func Open(hostRoot, journalDirectory string) (*Engine, error) {
	owner := os.Geteuid()

	host, err := protectedRoot(hostRoot, owner, false)
	if err != nil {
		return nil, err
	}
	jr, err := protectedRoot(journalDirectory, owner, true)
	if err != nil {
		host.Close()
		return nil, err
	}
	return openRoots(host, jr, owner)
}

func openRoots(host, jr *os.Root, owner int) (*Engine, error) {
	lock, err := jr.OpenFile("install.lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		host.Close()
		jr.Close()
		return nil, err
	}
	info, err := lock.Stat()
	if err != nil || !info.Mode().IsRegular() || !owned(info, owner) || info.Mode().Perm() != 0600 {
		lock.Close()
		host.Close()
		jr.Close()
		return nil, ErrConflict
	}
	if err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		lock.Close()
		host.Close()
		jr.Close()
		return nil, ErrConflict
	}
	return &Engine{host: host, journalRoot: jr, lock: lock, owner: owner}, nil
}
func protectedRoot(name string, owner int, private bool) (*os.Root, error) {
	before, err := os.Lstat(name)
	if err != nil || !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return nil, ErrConflict
	}
	root, err := os.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	directory, err := root.Open(".")
	if err != nil {
		root.Close()
		return nil, err
	}
	info, err := directory.Stat()
	_ = directory.Close()
	if err != nil || !os.SameFile(before, info) || !info.IsDir() || !owned(info, owner) || info.Mode().Perm()&0022 != 0 || (private && info.Mode().Perm()&0077 != 0) {
		root.Close()
		return nil, ErrConflict
	}
	return root, nil
}
func owned(info os.FileInfo, uid int) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == uid
}
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return errorsJoin(e.lock.Close(), e.host.Close(), e.journalRoot.Close())
}
func syncDirectory(root *os.Root, name string) error {
	file, err := root.OpenFile(name, os.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}
func (e *Engine) load() (journal, error) {
	var j journal
	file, err := e.journalRoot.OpenFile("install.json", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return j, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !owned(info, e.owner) || info.Mode().Perm() != 0600 {
		return j, ErrConflict
	}
	data, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	if err != nil || len(data) > 64<<10 {
		return j, ErrConflict
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&j) != nil || decoder.Decode(new(any)) != io.EOF || j.Version != 1 || len(j.Items) == 0 || len(j.Items) > 32 || len(j.ID) != 32 || len(j.Digest) != 64 {
		return j, ErrConflict
	}
	if _, err = hex.DecodeString(j.ID); err != nil {
		return j, ErrConflict
	}
	switch j.Phase {
	case "installing", "installed", "rolling-back", "rolled-back":
	default:
		return j, ErrConflict
	}
	seen := map[string]bool{}
	normalized := make([]record, 0, len(j.Items))
	for _, r := range j.Items {
		if !validRecord(r, e.owner) || seen[r.Path] {
			return j, ErrConflict
		}
		if (j.Phase == "installed" && r.State != "created" && r.State != "existing") || (j.Phase == "rolled-back" && r.State != "removed" && r.State != "existing") || (j.Phase == "installing" && r.State == "removed") {
			return j, ErrConflict
		}
		switch r.State {
		case "pending", "created", "existing", "removed":
		default:
			return j, ErrConflict
		}
		if !r.Directory && r.State == "existing" {
			return j, ErrConflict
		}
		seen[r.Path] = true
		r.State = "pending"
		normalized = append(normalized, r)
	}
	encoded, _ := json.Marshal(normalized)
	if digest(encoded) != j.Digest {
		return j, ErrConflict
	}
	return j, nil
}
func (e *Engine) save(j journal) error {
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return e.saveJournalBytes("install", data)
}
func (e *Engine) saveJournalBytes(name string, data []byte) error {
	if name != "install" && name != "accounts" && name != "images" && name != "maintenance-accounts" {
		return ErrPlan
	}
	next, final := name+".next", name+".json"
	var err error
	// This reserved staging name belongs only to this private journal. Recover
	// a torn previous journal write without touching the last committed journal.
	if info, err := e.journalRoot.Lstat(next); err == nil {
		if !info.Mode().IsRegular() || !owned(info, e.owner) || info.Mode().Perm() != 0600 {
			return ErrConflict
		}
		if err = e.journalRoot.Remove(next); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	file, err := e.journalRoot.OpenFile(next, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	err = errorsJoin(err, file.Close())
	if err != nil {
		return err
	}
	if err = e.journalRoot.Rename(next, final); err != nil {
		return err
	}
	return syncDirectory(e.journalRoot, ".")
}
func newID() (string, error) {
	data := make([]byte, 16)
	_, err := rand.Read(data)
	return hex.EncodeToString(data), err
}
