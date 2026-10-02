//go:build linux

package updates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/theupdateframework/go-tuf/v2/metadata"
)

func TestVerifiedPackageStreamAdmission(t *testing.T) {
	for _, scenario := range []string{"valid", "corrupt", "overflow", "short", "redirect", "public-staging", "existing-pending", "existing-verified"} {
		t.Run(scenario, func(t *testing.T) {
			payload := []byte("development package fixture")
			expected := sha256.Sum256(payload)
			digest := hex.EncodeToString(expected[:])
			target := &metadata.TargetFiles{Path: "homenode.deb", Length: int64(len(payload)), Hashes: metadata.Hashes{"sha256": expected[:]}}
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/targets/"+digest+".homenode.deb" {
					t.Error("wrong consistent target path", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				if scenario == "redirect" {
					http.Redirect(w, r, "/elsewhere", 302)
					return
				}
				w.WriteHeader(200)
				w.(http.Flusher).Flush()
				data := append([]byte(nil), payload...)
				switch scenario {
				case "corrupt":
					data[0] ^= 1
				case "overflow":
					data = append(data, 1)
				case "short":
					data = data[:len(data)-1]
				}
				_, _ = w.Write(data)
			}))
			defer server.Close()
			fetcher, err := newMetadataFetcher(context.Background(), server.URL+"/targets")
			if err != nil {
				t.Fatal(err)
			}
			fetcher.client.Transport = server.Client().Transport
			directory := t.TempDir()
			if err = os.Chmod(directory, 0700); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "public-staging":
				err = os.Chmod(directory, 0755)
			case "existing-pending":
				err = os.WriteFile(filepath.Join(directory, digest+".pending"), []byte("retained"), 0600)
			case "existing-verified":
				err = os.WriteFile(filepath.Join(directory, digest+".deb"), []byte("retained"), 0400)
			}
			if err != nil {
				t.Fatal(err)
			}
			root, err := os.OpenRoot(directory)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			file, err := acquireVerifiedPackage(context.Background(), fetcher, target, true, root)
			if scenario == "valid" {
				if err != nil {
					t.Fatal(err)
				}
				defer file.Close()
				data, err := io.ReadAll(file)
				if err != nil || string(data) != string(payload) {
					t.Fatal(string(data), err)
				}
				if _, err = file.Write([]byte("mutation")); err == nil {
					t.Fatal("verified descriptor writable")
				}
			} else {
				if file != nil {
					file.Close()
					t.Fatal("failed package returned authority")
				}
				if err == nil {
					t.Fatal("unsafe package accepted")
				}
				if scenario != "existing-verified" {
					if _, err = os.Lstat(filepath.Join(directory, digest+".deb")); !os.IsNotExist(err) {
						t.Fatal("failed package published", err)
					}
				}
			}
		})
	}
}

func TestPackageDownloadCancellationRetainsNoVerifiedAuthority(t *testing.T) {
	payload := []byte("expected package")
	sum := sha256.Sum256(payload)
	digest := hex.EncodeToString(sum[:])
	target := &metadata.TargetFiles{Path: "homenode.deb", Length: int64(len(payload)), Hashes: metadata.Hashes{"sha256": sum[:]}}
	started := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fetcher, err := newMetadataFetcher(ctx, server.URL+"/targets")
	if err != nil {
		t.Fatal(err)
	}
	fetcher.client.Transport = server.Client().Transport
	directory := t.TempDir()
	if err = os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	done := make(chan error, 1)
	go func() {
		file, err := acquireVerifiedPackage(ctx, fetcher, target, true, root)
		if file != nil {
			file.Close()
			done <- errors.New("canceled download returned a descriptor")
			return
		}
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("download did not start")
	}
	cancel()
	select {
	case err = <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled package download remained active")
	}
	if _, err = os.Lstat(filepath.Join(directory, digest+".deb")); !os.IsNotExist(err) {
		t.Fatal("canceled package published", err)
	}
	if file, err := acquireVerifiedPackage(ctx, fetcher, target, true, root); !errors.Is(err, context.Canceled) || file != nil {
		if file != nil {
			file.Close()
		}
		t.Fatal("canceled operation reused", err)
	}
}

func TestPackageCancellationWinsOverCleanEOF(t *testing.T) {
	payload := []byte("expected signed bytes")
	sum := sha256.Sum256(payload)
	target := &metadata.TargetFiles{Path: "homenode.deb", Length: int64(len(payload)), Hashes: metadata.Hashes{"sha256": sum[:]}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fetcher, err := newMetadataFetcher(ctx, "https://updates.example/targets/")
	if err != nil {
		t.Fatal(err)
	}
	fetcher.client.Transport = cancellationTransport{cancel}
	directory := t.TempDir()
	if err := os.Chmod(directory, 0700); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	file, err := acquireVerifiedPackage(ctx, fetcher, target, true, root)
	if file != nil {
		file.Close()
		t.Fatal("canceled EOF returned package authority")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation became length refusal", err)
	}
	if _, err := root.Lstat(hex.EncodeToString(sum[:]) + ".deb"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("canceled package published", err)
	}
}
