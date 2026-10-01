package backup

import (
	"errors"
	"os"
	"testing"
)

func TestStagingCleanupPreservesReplacement(t *testing.T) {
	for _, kind := range []string{"original", "replacement", "symlink", "missing"} {
		t.Run(kind, func(t *testing.T) {
			root, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			file, err := root.OpenFile("files.raw", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			info, err := file.Stat()
			if err != nil {
				t.Fatal(err)
			}
			created := map[string]os.FileInfo{"files.raw": info}
			switch kind {
			case "replacement":
				if err = root.Rename("files.raw", "original.raw"); err != nil {
					t.Fatal(err)
				}
				replacement, err := root.OpenFile("files.raw", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
				if err != nil {
					t.Fatal(err)
				}
				replacement.Write([]byte("preserve"))
				replacement.Close()
			case "symlink":
				if err = root.Rename("files.raw", "original.raw"); err != nil {
					t.Fatal(err)
				}
				if err = root.Symlink("original.raw", "files.raw"); err != nil {
					t.Fatal(err)
				}
			case "missing":
				if err = root.Remove("files.raw"); err != nil {
					t.Fatal(err)
				}
			}
			err = removeOwnedStaging(root, created)
			if kind == "replacement" || kind == "symlink" {
				if !errors.Is(err, ErrManifest) {
					t.Fatal("replacement not reported", err)
				}
				if _, err = root.Lstat("files.raw"); err != nil {
					t.Fatal("replacement deleted", err)
				}
				if _, err = root.Lstat("original.raw"); err != nil {
					t.Fatal("renamed original deleted", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if _, err = root.Lstat("files.raw"); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("owned partial payload retained", err)
				}
			}
		})
	}
}
