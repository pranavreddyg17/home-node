package install

import (
	"context"
	"errors"
	"io"
	"os"
	"syscall"
)

// observeRecoveryDestinationVacancy observes fixed, already ownership-verified
// directories. It preserves all existing entries and does not exclude writers;
// production publication must retain service/guest activation exclusion.
func (e *Engine) observeRecoveryDestinationVacancy(ctx context.Context) error {
	for _, name := range []string{"var/lib/homenode/control", "var/lib/homenode/supervisor", "var/lib/homenode/volumes"} {
		if err := recoveryDirectoryVacant(ctx, e.host, name); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func recoveryDirectoryVacant(ctx context.Context, root *os.Root, name string) (result error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch name {
	case "var/lib/homenode/control", "var/lib/homenode/supervisor", "var/lib/homenode/volumes":
	default:
		return ErrPlan
	}
	if root == nil {
		return ErrPlan
	}
	before, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return ErrConflict
	}
	directory, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, directory.Close()) }()
	opened, err := directory.Stat()
	if err != nil || !opened.IsDir() || !os.SameFile(before, opened) {
		return ErrConflict
	}
	// A single entry suffices to refuse; never enumerate an unbounded directory.
	entries, err := directory.ReadDir(1)
	if len(entries) != 0 || !errors.Is(err, io.EOF) {
		return errors.Join(ErrConflict, err)
	}
	current, err := root.Lstat(name)
	if err != nil || !os.SameFile(opened, current) || !current.IsDir() {
		return ErrConflict
	}
	return ctx.Err()
}
