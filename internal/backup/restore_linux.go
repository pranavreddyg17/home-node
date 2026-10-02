//go:build linux

package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strconv"
	"time"
)

func (r *Repository) dump(ctx context.Context, snapshot, name string, output io.Writer) error {
	args := []string{"--repo", "/proc/self/fd/3", "--password-file", "/proc/self/fd/4", "--no-cache", "dump", snapshot, "/proc/self/fd/5/" + name}
	return resticProcess(ctx, args, []*os.File{r.directory, r.secret}, output)
}

// Restore retrieves only fixed recovery-set filenames into empty private staging.
// It verifies bytes, schema and removed authority before reporting success. It
// does not install disks, configure identity, or activate recovered applications.
func (r *Repository) Restore(ctx context.Context, snapshot string, stage *os.File, policy RestorePolicy) (manifest Manifest, resultErr error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Manifest{}, err
	}
	if r.directory == nil || stage == nil || !repositoryPattern.MatchString(snapshot) {
		return Manifest{}, ErrRepository
	}
	deadline, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	root, err := os.OpenRoot("/proc/self/fd/" + strconv.FormatUint(uint64(stage.Fd()), 10))
	if err != nil {
		return Manifest{}, ErrManifest
	}
	defer root.Close()
	info, err := root.Stat(".")
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return Manifest{}, ErrManifest
	}
	directory, err := root.Open(".")
	if err != nil {
		return Manifest{}, ErrManifest
	}
	entries, readErr := directory.ReadDir(1)
	_ = directory.Close()
	if len(entries) != 0 || readErr != io.EOF {
		return Manifest{}, ErrManifest
	}
	output := &boundedOutput{maximum: MaxManifestBytes}
	if err = r.dump(deadline, snapshot, "manifest.json", output); err != nil {
		return Manifest{}, err
	}
	manifest, err = DecodeManifest(output.data)
	if err != nil || manifest.Validate(policy, time.Now()) != nil {
		return Manifest{}, ErrManifest
	}
	created := map[string]os.FileInfo{}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, removeOwnedStaging(root, created))
			resultErr = errors.Join(resultErr, stage.Sync())
		}
	}()
	for _, entry := range manifest.Files {
		if err := deadline.Err(); err != nil {
			return Manifest{}, err
		}
		if err := requireStagingSpace(stage, entry.Bytes); err != nil {
			return Manifest{}, err
		}
		file, err := root.OpenFile(entry.Name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return Manifest{}, ErrManifest
		}
		info, statErr := file.Stat()
		if statErr != nil {
			return Manifest{}, errors.Join(statErr, file.Close())
		}
		created[entry.Name] = info
		hash := sha256.New()
		writer := &restoreWriter{ctx: deadline, stage: stage, destination: io.MultiWriter(file, hash), remaining: entry.Bytes}
		err = r.dump(deadline, snapshot, entry.Name, writer)
		if err == nil && (writer.remaining != 0 || hex.EncodeToString(hash.Sum(nil)) != entry.SHA256) {
			err = ErrManifest
		}
		err = errors.Join(err, file.Sync(), file.Close())
		if err != nil {
			return Manifest{}, err
		}
	}
	if err := deadline.Err(); err != nil {
		return Manifest{}, err
	}
	if err := requireStagingSpace(stage, int64(len(output.data))); err != nil {
		return Manifest{}, err
	}
	file, err := root.OpenFile("manifest.json", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return Manifest{}, ErrManifest
	}
	info, statErr := file.Stat()
	if statErr != nil {
		return Manifest{}, errors.Join(statErr, file.Close())
	}
	created["manifest.json"] = info
	_, err = file.Write(output.data)
	err = errors.Join(err, file.Sync(), file.Close())
	if err != nil {
		return Manifest{}, err
	}
	if err = ValidateRecoverySet(deadline, root, manifest, policy); err != nil {
		return Manifest{}, err
	}
	if err = stage.Sync(); err != nil {
		return Manifest{}, err
	}
	if err := deadline.Err(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}
