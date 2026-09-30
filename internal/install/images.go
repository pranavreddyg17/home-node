package install

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/pranavreddyg17/home-node/internal/catalog"
)

type imageJournal struct {
	Version         int    `json:"version"`
	ConfigurationID string `json:"configurationId"`
	CatalogDigest   string `json:"catalogDigest"`
	Completed       int    `json:"completed"`
}

// PlaceImages publishes only artifacts authenticated by independent release
// trust and already committed configuration. It never boots a guest or starts
// a service. source contains fixed SHA256.raw filenames, not executable tools.
func (e *Engine) PlaceImages(ctx context.Context, source string, publisher ed25519.PublicKey, floor int64) error {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || e.host.Name() != "/" {
		return ErrConflict
	}
	return e.placeImages(ctx, source, publisher, floor, time.Now())
}

func (e *Engine) placeImages(ctx context.Context, source string, publisher ed25519.PublicKey, floor int64, now time.Time) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	config, err := e.load()
	if err != nil {
		return err
	}
	if config.Phase != "installed" {
		return ErrConflict
	}
	var gid int
	for _, r := range config.Items {
		if err = e.matches(r); err != nil {
			return ErrConflict
		}
		if r.Path == "var/lib/homenode/images" {
			gid = r.GID
		}
	}
	if gid <= 0 || len(publisher) != ed25519.PublicKeySize || floor < 1 {
		return ErrPlan
	}
	// These bytes are checked against the installation journal before use.
	data, err := e.readConfiguration(config, "var/lib/homenode/catalog/catalog.json")
	if err != nil || len(data) > catalog.MaxManifestBytes {
		return ErrConflict
	}
	pub, err := e.readConfiguration(config, "etc/homenode/catalog.pub")
	if err != nil || string(pub) != hex.EncodeToString(publisher)+"\n" {
		return ErrConflict
	}
	manifest, err := catalog.Verify(data, map[string]ed25519.PublicKey{catalog.KeyID(publisher): publisher}, floor, now)
	if err != nil || len(manifest.Images) != 3 {
		return catalog.ErrUntrusted
	}
	images, err := protectedChildPath(e.host, "var/lib/homenode/images", e.owner, gid)
	if err != nil {
		return err
	}
	defer images.Close()
	input, err := os.OpenRoot(source)
	if err != nil {
		return err
	}
	defer input.Close()
	j, err := e.loadImageJournal()
	if os.IsNotExist(err) {
		// Occupied images are not adopted merely because their content matches.
		for _, image := range manifest.Images {
			if _, err = images.Lstat(image.SHA256 + ".raw"); err == nil {
				return ErrConflict
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		j = imageJournal{Version: 1, ConfigurationID: config.ID, CatalogDigest: digest(data)}
		if err = e.saveImageJournal(j); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if j.ConfigurationID != config.ID || j.CatalogDigest != digest(data) || j.Completed > len(manifest.Images) {
		return ErrConflict
	}
	for index, image := range manifest.Images {
		if err = ctx.Err(); err != nil {
			return err
		}
		final := image.SHA256 + ".raw"
		stage := ".homenode-" + config.ID + "-image-" + strconv.Itoa(index) + ".stage"
		if _, err = images.Lstat(final); err == nil {
			if err = verifyPlacedImage(ctx, images, final, image, e.owner, gid); err != nil {
				return err
			}
		} else {
			if !os.IsNotExist(err) {
				return err
			}
			if index < j.Completed {
				return ErrConflict
			}
			if err = removeImageStage(images, stage, e.owner); err != nil {
				return err
			}
			if err = copyImage(ctx, input, images, stage, image, e.owner, gid); err != nil {
				return err
			}
			if e.checkpoint != nil {
				if err = e.checkpoint("image-staged", image.ID); err != nil {
					return err
				}
			}
			if err = images.Link(stage, final); err != nil {
				return err
			}
			if err = syncDirectory(images, "."); err != nil {
				return err
			}
			if e.checkpoint != nil {
				if err = e.checkpoint("image-published", image.ID); err != nil {
					return err
				}
			}
		}
		if err = removeImageStage(images, stage, e.owner); err != nil {
			return err
		}
		if err = syncDirectory(images, "."); err != nil {
			return err
		}
		if index >= j.Completed {
			j.Completed = index + 1
			if err = e.saveImageJournal(j); err != nil {
				return err
			}
		}
	}
	return nil
}

// The image directory is deliberately traversable only by the QEMU group.
func protectedChildPath(host *os.Root, name string, owner, gid int) (*os.Root, error) {
	before, err := host.Lstat(name)
	if err != nil || !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return nil, ErrConflict
	}
	root, err := host.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	f, err := root.Open(".")
	if err != nil {
		root.Close()
		return nil, err
	}
	info, err := f.Stat()
	f.Close()
	if err != nil || !os.SameFile(before, info) || !owned(info, owner) || info.Mode().Perm() != 0710 {
		root.Close()
		return nil, ErrConflict
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Gid) != gid {
		root.Close()
		return nil, ErrConflict
	}
	return root, nil
}
func (e *Engine) loadImageJournal() (imageJournal, error) {
	var j imageJournal
	f, err := e.journalRoot.OpenFile("images.json", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return j, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || !owned(info, e.owner) || info.Mode().Perm() != 0600 || info.Size() > 64<<10 {
		return j, ErrConflict
	}
	data, err := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err != nil || len(data) > 64<<10 {
		return j, ErrConflict
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(&j) != nil || d.Decode(new(any)) != io.EOF || j.Version != 1 || j.Completed < 0 || j.Completed > 3 || len(j.ConfigurationID) != 32 || len(j.CatalogDigest) != 64 {
		return j, ErrConflict
	}
	if _, err = hex.DecodeString(j.ConfigurationID); err != nil {
		return j, ErrConflict
	}
	if _, err = hex.DecodeString(j.CatalogDigest); err != nil {
		return j, ErrConflict
	}
	return j, nil
}
func (e *Engine) saveImageJournal(j imageJournal) error {
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	return e.saveJournalBytes("images", data)
}
func removeImageStage(root *os.Root, name string, owner int) error {
	info, err := root.Lstat(name)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || !owned(info, owner) || info.Mode().Perm()&0037 != 0 {
		return ErrConflict
	}
	return root.Remove(name)
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
func copyImage(ctx context.Context, input, output *os.Root, stage string, image catalog.Image, owner, gid int) error {
	f, err := input.OpenFile(image.SHA256+".raw", os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != image.Bytes {
		return catalog.ErrUntrusted
	}
	out, err := output.OpenFile(stage, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, h), io.LimitReader(contextReader{ctx, f}, image.Bytes+1))
	if err != nil {
		return err
	}
	if n != image.Bytes || hex.EncodeToString(h.Sum(nil)) != image.SHA256 {
		return catalog.ErrUntrusted
	}
	if err = out.Chown(owner, gid); err != nil {
		return err
	}
	if err = out.Chmod(0440); err != nil {
		return err
	}
	if err = out.Sync(); err != nil {
		return err
	}
	return out.Close()
}
func verifyPlacedImage(ctx context.Context, root *os.Root, name string, image catalog.Image, owner, gid int) error {
	f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || !owned(info, owner) || info.Mode().Perm() != 0440 || info.Size() != image.Bytes {
		return ErrConflict
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(st.Gid) != gid {
		return ErrConflict
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(contextReader{ctx, f}, image.Bytes+1))
	if err != nil {
		return err
	}
	if n != image.Bytes || hex.EncodeToString(h.Sum(nil)) != image.SHA256 {
		return catalog.ErrUntrusted
	}
	return nil
}

func (e *Engine) readConfiguration(j journal, name string) ([]byte, error) {
	var expected string
	for _, r := range j.Items {
		if r.Path == name && !r.Directory {
			expected = r.SHA256
		}
	}
	if expected == "" {
		return nil, ErrConflict
	}
	f, err := e.host.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, ErrConflict
	}
	data, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	if err != nil || len(data) > maxFileBytes || digest(data) != expected {
		return nil, ErrConflict
	}
	return data, nil
}
