package install

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/catalog"
)

func imagePlacementFixture(t *testing.T) (*Engine, Configuration, string, string, string, time.Time) {
	t.Helper()
	c, m, key, now := configurationFixture(t)
	source := t.TempDir()
	for index := range m.Images {
		data := bytes.Repeat([]byte(m.Images[index].ID), 256)
		m.Images[index].SHA256 = digest(data)
		m.Images[index].Bytes = int64(len(data))
		if err := os.WriteFile(filepath.Join(source, m.Images[index].SHA256+".raw"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	c.Catalog = signConfigurationCatalog(t, m, c.Publisher, key)
	p, err := ConfigurationPlan(c, now)
	if err != nil {
		t.Fatal(err)
	}
	host, jr := roots(t)
	e := openEngine(t, host, jr)
	if err = e.Apply(context.Background(), p.Plan); err != nil {
		e.Close()
		t.Fatal(err)
	}
	return e, c, host, jr, source, now
}

func TestRootImagePlacementResumeAfterPublication(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-only temporary fixture")
	}
	e, c, host, jr, source, now := imagePlacementFixture(t)
	stopped := errors.New("lost publication acknowledgement")
	e.checkpoint = func(stage, name string) error {
		if stage == "image-published" {
			return stopped
		}
		return nil
	}
	if err := e.placeImages(context.Background(), source, c.Publisher, c.MinimumCatalogVersion, now); !errors.Is(err, stopped) {
		t.Fatal(err)
	}
	j, err := e.loadImageJournal()
	if err != nil || j.Completed != 0 {
		t.Fatal(j, err)
	}
	e.Close()
	e = openEngine(t, host, jr)
	defer e.Close()
	if err = e.placeImages(context.Background(), source, c.Publisher, c.MinimumCatalogVersion, now); err != nil {
		t.Fatal(err)
	}
	j, err = e.loadImageJournal()
	if err != nil || j.Completed != 3 {
		t.Fatal(j, err)
	}
	if err = e.placeImages(context.Background(), source, c.Publisher, c.MinimumCatalogVersion, now); err != nil {
		t.Fatal("replay", err)
	}
	// Image data deliberately prevents configuration rollback from erasing it.
	if err = e.Rollback(context.Background()); err == nil {
		t.Fatal("rollback removed occupied image directory")
	}
}

func TestRootImagePlacementRefusesForeignAndChangedArtifacts(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-only temporary fixture")
	}
	for _, which := range []string{"occupied", "symlink", "corrupt-source", "missing-completed", "changed-published", "wrong-key"} {
		t.Run(which, func(t *testing.T) {
			e, c, host, _, source, now := imagePlacementFixture(t)
			defer e.Close()
			names, err := os.ReadDir(source)
			if err != nil {
				t.Fatal(err)
			}
			name := names[0].Name()
			src := filepath.Join(source, name)
			dst := filepath.Join(host, "var/lib/homenode/images", name)
			switch which {
			case "occupied":
				data, err := os.ReadFile(src)
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(dst, data, 0440); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err = os.Rename(src, src+".original"); err != nil {
					t.Fatal(err)
				}
				if err = os.Symlink(src+".original", src); err != nil {
					t.Fatal(err)
				}
			case "corrupt-source":
				data, err := os.ReadFile(src)
				if err != nil {
					t.Fatal(err)
				}
				data[0] ^= 1
				if err = os.WriteFile(src, data, 0600); err != nil {
					t.Fatal(err)
				}
			case "missing-completed", "changed-published":
				if err = e.placeImages(context.Background(), source, c.Publisher, c.MinimumCatalogVersion, now); err != nil {
					t.Fatal(err)
				}
				if which == "missing-completed" {
					err = os.Remove(dst)
				} else {
					err = os.Chmod(dst, 0640)
				}
				if err != nil {
					t.Fatal(err)
				}
			case "wrong-key":
				c.Publisher = bytes.Repeat([]byte{7}, 32)
			}
			if err = e.placeImages(context.Background(), source, c.Publisher, c.MinimumCatalogVersion, now); err == nil {
				t.Fatal("unsafe image placement accepted")
			}
			if which == "occupied" || which == "wrong-key" {
				if _, err = e.loadImageJournal(); !os.IsNotExist(err) {
					t.Fatal("ownership intent recorded for foreign input", err)
				}
			}
		})
	}
}

func TestStreamedImageCopyIntegrityAndCancellation(t *testing.T) {
	for _, which := range []string{"valid", "corrupt", "cancelled", "symlink"} {
		t.Run(which, func(t *testing.T) {
			inputPath, outputPath := t.TempDir(), t.TempDir()
			data := bytes.Repeat([]byte("immutable release fixture"), 8192)
			image := catalog.Image{SHA256: digest(data), Bytes: int64(len(data))}
			name := filepath.Join(inputPath, image.SHA256+".raw")
			if which == "corrupt" {
				data[0] ^= 1
			}
			if err := os.WriteFile(name, data, 0600); err != nil {
				t.Fatal(err)
			}
			if which == "symlink" {
				if err := os.Rename(name, name+".original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(name+".original", name); err != nil {
					t.Fatal(err)
				}
			}
			input, err := os.OpenRoot(inputPath)
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			output, err := os.OpenRoot(outputPath)
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if which == "cancelled" {
				cancel()
			}
			err = copyImage(ctx, input, output, "stage", image, os.Geteuid(), os.Getegid())
			if which == "valid" {
				if err != nil {
					t.Fatal(err)
				}
				if err = verifyPlacedImage(ctx, output, "stage", image, os.Geteuid(), os.Getegid()); err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("invalid stream accepted")
			}
		})
	}
}

func TestRootConfigurationReplayCreditsOnlyOwnedImages(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root-only temporary fixture")
	}
	for _, which := range []string{"published", "staged", "foreign", "changed"} {
		t.Run(which, func(t *testing.T) {
			e, c, host, _, source, now := imagePlacementFixture(t)
			defer e.Close()
			readyAccountIntent(t, e, c.Accounts, true)
			initial, err := ConfigurationPlan(c, now)
			if err != nil {
				t.Fatal(err)
			}
			if which == "staged" {
				stopped := errors.New("staging result lost")
				e.checkpoint = func(stage, name string) error {
					if stage == "image-staged" {
						return stopped
					}
					return nil
				}
				if err = e.placeImages(context.Background(), source, c.Publisher, c.MinimumCatalogVersion, now); !errors.Is(err, stopped) {
					t.Fatal(err)
				}
				e.checkpoint = nil
			} else if which != "foreign" {
				if err = e.placeImages(context.Background(), source, c.Publisher, c.MinimumCatalogVersion, now); err != nil {
					t.Fatal(err)
				}
			}
			entries, err := os.ReadDir(source)
			if err != nil {
				t.Fatal(err)
			}
			if which == "foreign" {
				data, err := os.ReadFile(filepath.Join(source, entries[0].Name()))
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(filepath.Join(host, "var/lib/homenode/images", entries[0].Name()), data, 0440); err != nil {
					t.Fatal(err)
				}
			}
			if which == "changed" {
				if err = os.Chmod(filepath.Join(host, "var/lib/homenode/images", entries[0].Name()), 0640); err != nil {
					t.Fatal(err)
				}
			}
			var total uint64
			for _, entry := range entries {
				if filepath.Ext(entry.Name()) == ".raw" {
					info, err := entry.Info()
					if err != nil {
						t.Fatal(err)
					}
					total += uint64(info.Size())
				}
			}
			observed := c.Capacity
			observed.FreeDiskBytes = initial.RequiredDiskBytes - total
			calls := 0
			observe := func(context.Context) (Accounts, Capacity, error) {
				calls++
				if which == "staged" && calls > 1 {
					observed.FreeDiskBytes = initial.RequiredDiskBytes
				}
				return c.Accounts, observed, nil
			}
			preview, err := e.configure(context.Background(), c, now, observe)
			if which == "foreign" || which == "changed" {
				if err == nil {
					t.Fatal("unowned/changed bytes credited")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if preview.ProvidedCapacity.FreeDiskBytes != observed.FreeDiskBytes {
				t.Fatal("measurement replaced")
			}
			if which == "published" && (preview.VerifiedImageBytes != total || preview.RequiredFreeDiskBytes != initial.RequiredDiskBytes-total) {
				t.Fatal("incorrect credit", preview)
			}
			if which == "staged" && (calls != 2 || preview.VerifiedImageBytes != 0) {
				t.Fatal("staging counted without fresh measurement", calls, preview)
			}
		})
	}
}
