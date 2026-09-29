//go:build linux

package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestRepositoryDirectoryRefusesSymlink(t *testing.T) {
	parentPath := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(parentPath, "homenode-backup")); err != nil {
		t.Fatal(err)
	}
	parent, err := os.Open(parentPath)
	if err != nil {
		t.Fatal(err)
	}
	defer parent.Close()
	if repo, err := repositoryDirectory(parent, false); err == nil {
		repo.Close()
		t.Fatal("symlink repository admitted")
	}
}

func TestPasswordDescriptorIsSealedAndAnonymous(t *testing.T) {
	password := []byte("a private test password")
	file, err := passwordDescriptor(password)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil || !bytes.Equal(data, password) {
		t.Fatal("password descriptor", err)
	}
	if _, err = file.WriteAt([]byte("x"), 0); err == nil {
		t.Fatal("password memory is writable")
	}
	for _, invalid := range [][]byte{nil, []byte("new\nline"), []byte("nul\x00byte"), make([]byte, 8193)} {
		if descriptor, err := passwordDescriptor(invalid); err == nil {
			descriptor.Close()
			t.Fatal("invalid password accepted")
		}
	}
}

func TestRealResticRepositoryAuthentication(t *testing.T) {
	if os.Getenv("HOMENODE_RESTIC_INTEGRATION") != "1" {
		t.Skip("requires disposable Linux restic integration environment")
	}
	directory, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	password := []byte("repository fixture password")
	// Initialize only this disposable test repository; no registered-drive writes.
	if _, err = resticConfig(context.Background(), directory, password, true); err != nil {
		t.Fatal("real restic init", err)
	}
	data, err := resticConfig(context.Background(), directory, password, false)
	if err != nil {
		t.Fatal("real restic config", err)
	}
	var config struct {
		ID string `json:"id"`
	}
	if err = json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	target := registeredTarget()
	target.RepositoryID = config.ID
	if err = VerifyRepository(context.Background(), directory, target, password); err != nil {
		t.Fatal(err)
	}
	target.RepositoryID = string(bytes.Repeat([]byte("0"), 64))
	if err = VerifyRepository(context.Background(), directory, target, password); err == nil {
		t.Fatal("wrong registered repository admitted")
	}
	target.RepositoryID = config.ID
	if err = VerifyRepository(context.Background(), directory, target, []byte("wrong password")); err == nil {
		t.Fatal("wrong password admitted")
	}
	repository, err := openRepository(context.Background(), directory, target, password)
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if err = os.Rename(filepath.Join(directory.Name(), "homenode-backup"), filepath.Join(directory.Name(), "original")); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(filepath.Join(directory.Name(), "homenode-backup"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = repository.Check(context.Background()); err != nil {
		t.Fatal("pinned repository check followed replacement", err)
	}
	if err = VerifyRepository(context.Background(), directory, target, password); err == nil {
		t.Fatal("replacement directory authenticated")
	}
	if err = repository.Close(); err != nil {
		t.Fatal(err)
	}
	if err = repository.Check(context.Background()); err == nil {
		t.Fatal("closed repository retained credentials")
	}
}
