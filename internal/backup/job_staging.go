package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// JobStaging retains an exclusive kernel lease on a private backup-owned parent
// and a pinned newly created job directory. Close releases handles, never data.
// Existing job directories are refused, including after a crash; reconciliation
// and protected disposal must inspect them rather than rerun uncertain work.
type JobStaging struct {
	root, parent *os.Root
	lock         *os.File
	once         sync.Once
	closeErr     error
}

func (s *JobStaging) Root() *os.Root {
	if s == nil {
		return nil
	}
	return s.root
}
func (s *JobStaging) Close() error {
	if s == nil {
		return nil
	}
	s.once.Do(func() { s.closeErr = errors.Join(s.root.Close(), s.parent.Close(), s.lock.Close()) })
	return s.closeErr
}

// OpenJobStaging requires a pre-provisioned private directory owned by the
// running backup UID. Its persistent lock inode serializes all job staging in
// that parent. The job name is an ID, never a dispatch-supplied filesystem path.
func OpenJobStaging(ctx context.Context, directory, jobID string) (*JobStaging, error) {
	if !filepath.IsAbs(directory) || filepath.Clean(directory) != directory || !guestproto.ValidID(jobID) {
		return nil, ErrManifest
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	parent, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	lock, err := lockPrivateRunnerRoot(ctx, parent)
	if err != nil {
		parent.Close()
		return nil, err
	}
	accepted := false
	defer func() {
		if !accepted {
			parent.Close()
			lock.Close()
		}
	}()
	if err = parent.Mkdir(jobID, 0700); err != nil {
		return nil, err
	}
	original, err := parent.Lstat(jobID)
	if err != nil || !original.IsDir() || original.Mode().Perm() != 0700 {
		return nil, ErrManifest
	}
	root, err := parent.OpenRoot(jobID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if !accepted {
			root.Close()
		}
	}()
	pinned, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	info, statErr := pinned.Stat()
	pinned.Close()
	current, currentErr := parent.Lstat(jobID)
	if statErr != nil || currentErr != nil || !os.SameFile(original, info) || !os.SameFile(original, current) {
		return nil, ErrManifest
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	accepted = true
	return &JobStaging{root: root, parent: parent, lock: lock}, nil
}
