package install

import (
	"os"
	"runtime"
)

// OpenSystem creates only the fixed private installation journal directory.
// Existing directories are validated, never repaired or recursively created.
func OpenSystem() (*Engine, error) {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return nil, ErrConflict
	}
	return bootstrap("/")
}

func bootstrap(hostPath string) (*Engine, error) {
	owner := os.Geteuid()
	host, err := protectedRoot(hostPath, owner, false)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*Engine, error) { host.Close(); return nil, err }
	parent := host
	var opened []*os.Root
	defer func() {
		for _, r := range opened {
			r.Close()
		}
	}()
	for _, component := range []string{"var", "lib"} {
		next, err := protectedChild(parent, component, owner, false)
		if err != nil {
			return fail(err)
		}
		opened = append(opened, next)
		parent = next
	}
	if err := parent.Mkdir("homenode-install", 0700); err != nil && !os.IsExist(err) {
		return fail(err)
	}
	// Sync even on replay: the previous creator may have exited before syncing.
	if err := syncDirectory(parent, "."); err != nil {
		return fail(err)
	}
	journal, err := protectedChild(parent, "homenode-install", owner, true)
	if err != nil {
		return fail(err)
	}
	return openRoots(host, journal, owner)
}

func protectedChild(parent *os.Root, name string, owner int, private bool) (*os.Root, error) {
	before, err := parent.Lstat(name)
	if err != nil || !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return nil, ErrConflict
	}
	child, err := parent.OpenRoot(name)
	if err != nil {
		return nil, ErrConflict
	}
	file, err := child.Open(".")
	if err != nil {
		child.Close()
		return nil, ErrConflict
	}
	info, err := file.Stat()
	file.Close()
	if err != nil || !os.SameFile(before, info) || !owned(info, owner) || info.Mode().Perm()&0022 != 0 || (private && info.Mode().Perm() != 0700) {
		child.Close()
		return nil, ErrConflict
	}
	return child, nil
}
