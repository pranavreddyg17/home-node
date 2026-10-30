//go:build linux

package supervisor

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
	"golang.org/x/sys/unix"
)

func TestNativeDomainPIDMetadata(t *testing.T) {
	if os.Getenv("HOMENODE_KVM_DOMAIN_INTEGRATION") != "1" || os.Getenv("HOMENODE_PID_METADATA_INTEGRATION") != "1" {
		t.Skip("explicit disposable root PID fixture required")
	}
	if os.Geteuid() != 0 {
		t.Fatal("root fixture identity required")
	}
	for _, kind := range []string{"valid", "owner", "group", "mode", "links", "empty", "oversized", "symlink", "fifo", "directory", "value"} {
		t.Run(kind, func(t *testing.T) {
			d := Domain{ID: state.Random()}
			base := t.TempDir()
			name := d.Name() + ".pid"
			path := filepath.Join(base, name)
			if err := os.WriteFile(path, []byte("12345\n"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.Chown(path, 0, 0); err != nil {
				t.Fatal(err)
			}
			var err error
			switch kind {
			case "owner":
				err = os.Chown(path, 200000, 0)
			case "group":
				err = os.Chown(path, 0, 108)
			case "mode":
				err = os.Chmod(path, 0664)
			case "links":
				err = os.Link(path, path+".alias")
			case "empty":
				err = os.WriteFile(path, nil, 0644)
			case "oversized":
				err = os.WriteFile(path, make([]byte, 33), 0644)
			case "value":
				err = os.WriteFile(path, []byte("+1\n"), 0644)
			case "symlink", "fifo", "directory":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "symlink":
					err = os.Symlink("/dev/null", path)
				case "fifo":
					err = unix.Mkfifo(path, 0644)
				case "directory":
					err = os.Mkdir(path, 0644)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			directory, err := os.Open(base)
			if err != nil {
				t.Fatal(err)
			}
			defer directory.Close()
			pid, err := readDomainPIDAt(int(directory.Fd()), d)
			if kind == "valid" {
				if err != nil || pid != 12345 {
					t.Fatal(pid, err)
				}
			} else if pid != 0 || !errors.Is(err, ErrPolicy) {
				t.Fatal("unqualified PID metadata accepted", kind, pid, err)
			}
		})
	}
	t.Log("native bounded PID file metadata refusal cases completed")
}
