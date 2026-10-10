package guest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
	"golang.org/x/sys/unix"
)

func TestObjectRequestsRefuseUnsafeEntriesWithoutReplacingEvidence(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "writable", "directory", "fifo"} {
		for _, suffix := range []string{".part", ".blob"} {
			t.Run(kind+suffix, func(t *testing.T) {
				directory := privateDataDir(t)
				a, err := New(directory, "files", 1<<30)
				if err != nil {
					t.Fatal(err)
				}
				defer a.Close()
				id := state.Random()
				path := filepath.Join(directory, id+suffix)
				target := filepath.Join(t.TempDir(), "preserved")
				data := []byte("original evidence")
				if err := os.WriteFile(target, data, 0600); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "symlink":
					err = os.Symlink(target, path)
				case "hardlink":
					err = os.Link(target, path)
				case "writable":
					err = os.WriteFile(path, data, 0644)
				case "directory":
					err = os.Mkdir(path, 0700)
				case "fifo":
					err = unix.Mkfifo(path, 0600)
				}
				if err != nil {
					t.Fatal(err)
				}
				before, err := os.Lstat(path)
				if err != nil {
					t.Fatal(err)
				}
				operations := []string{"upload", "stat", "finalize"}
				if suffix == ".blob" {
					operations = []string{"download", "stat", "finalize"}
					// A valid partial object must not overwrite a foreign final entry.
					if err := os.WriteFile(filepath.Join(directory, id+".part"), data, 0600); err != nil {
						t.Fatal(err)
					}
					// stat legitimately reports the private partial entry first.
					operations = []string{"download", "finalize"}
				}
				for _, operation := range operations {
					r := guestproto.Request{Version: 1, RequestID: state.Random(), Operation: operation, ObjectID: id, Data: data, Size: int64(len(data)), SHA256: checksum(data)}
					if response := a.Handle(r); response.Error == "" {
						t.Fatal("unsafe object admitted", operation, response)
					}
					current, err := os.Lstat(path)
					if err != nil || !os.SameFile(before, current) || current.Mode() != before.Mode() {
						t.Fatal("refusal replaced evidence", operation, err)
					}
				}
				if actual, err := os.ReadFile(target); err != nil || string(actual) != string(data) {
					t.Fatal("refusal changed linked target", err)
				}
			})
		}
	}
}
