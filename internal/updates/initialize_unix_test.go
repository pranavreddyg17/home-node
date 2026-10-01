//go:build linux || darwin

package updates

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sigstore/sigstore/pkg/signature"
	"github.com/theupdateframework/go-tuf/v2/metadata"
)

func thresholdBootstrap(t *testing.T) BootstrapRoot {
	t.Helper()
	root := metadata.Root(time.Now().UTC().Add(time.Hour))
	root.Signed.Roles[metadata.ROOT].Threshold = 2
	var signers []signature.Signer
	for _, role := range []string{metadata.ROOT, metadata.TIMESTAMP, metadata.SNAPSHOT, metadata.TARGETS} {
		count := 1
		if role == metadata.ROOT {
			count = 2
		}
		for i := 0; i < count; i++ {
			public, private, err := ed25519.GenerateKey(rand.Reader)
			if err != nil {
				t.Fatal(err)
			}
			key, err := metadata.KeyFromPublicKey(public)
			if err != nil {
				t.Fatal(err)
			}
			if err = root.Signed.AddKey(key, role); err != nil {
				t.Fatal(err)
			}
			if role == metadata.ROOT {
				signer, err := signature.LoadSigner(private, crypto.Hash(0))
				if err != nil {
					t.Fatal(err)
				}
				signers = append(signers, signer)
			}
		}
	}
	for _, signer := range signers {
		if _, err := root.Sign(signer); err != nil {
			t.Fatal(err)
		}
	}
	data, err := root.ToBytes(false)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return BootstrapRoot{Data: data, SHA256: hex.EncodeToString(sum[:])}
}

func TestCacheRootInitializationResumesRecordedIntent(t *testing.T) {
	for _, stage := range []string{"intent", "root", "none"} {
		t.Run(stage, func(t *testing.T) {
			provisioned, directory := updateCacheFixture(t)
			bootstrap := thresholdBootstrap(t)
			failure := errors.New("interrupted fixture")
			err := initializeCacheRoot(context.Background(), provisioned, bootstrap, func(current string) error {
				if current == stage {
					return failure
				}
				return nil
			})
			if stage != "none" && !errors.Is(err, failure) {
				t.Fatal(err)
			}
			if stage == "none" && err != nil {
				t.Fatal(err)
			}
			reopened, err := os.OpenRoot(directory)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			if err = InitializeCacheRoot(context.Background(), reopened, bootstrap); err != nil {
				t.Fatal("resume failed", err)
			}
			if err = InitializeCacheRoot(context.Background(), reopened, bootstrap); err != nil {
				t.Fatal("completed initialization not idempotent", err)
			}
			current, err := os.ReadFile(filepath.Join(directory, "metadata", "root.json"))
			if err != nil || string(current) != string(bootstrap.Data) {
				t.Fatal("wrong initialized root", err)
			}
			if _, err = os.Lstat(filepath.Join(directory, "bootstrap.pending")); !os.IsNotExist(err) {
				t.Fatal("intent not completed", err)
			}
		})
	}
}

func TestCacheRootInitializationNeverAdoptsOrResetsState(t *testing.T) {
	for _, scenario := range []string{"unowned-root", "old-timestamp", "clock", "wrong-intent", "missing-completed-root", "changed-pin", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			provisioned, directory := updateCacheFixture(t)
			bootstrap := thresholdBootstrap(t)
			var err error
			switch scenario {
			case "unowned-root":
				err = os.WriteFile(filepath.Join(directory, "metadata", "root.json"), bootstrap.Data, 0600)
			case "old-timestamp":
				err = os.WriteFile(filepath.Join(directory, "metadata", "timestamp.json"), []byte("retained"), 0600)
			case "clock":
				err = os.WriteFile(filepath.Join(directory, "clock"), []byte("retained"), 0600)
			case "wrong-intent":
				err = os.WriteFile(filepath.Join(directory, "bootstrap.pending"), []byte("other\n"), 0600)
			case "missing-completed-root", "changed-pin":
				err = InitializeCacheRoot(context.Background(), provisioned, bootstrap)
				if err == nil && scenario == "missing-completed-root" {
					err = os.Remove(filepath.Join(directory, "metadata", "root.json"))
				}
				if scenario == "changed-pin" {
					bootstrap = thresholdBootstrap(t)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario == "canceled" {
				cancel()
			}
			if err = InitializeCacheRoot(ctx, provisioned, bootstrap); err == nil {
				t.Fatal("existing/lost authority reset")
			}
			if scenario == "missing-completed-root" {
				if _, err = os.Lstat(filepath.Join(directory, "metadata", "root.json")); !os.IsNotExist(err) {
					t.Fatal("lost current root silently restored", err)
				}
			}
		})
	}
}
