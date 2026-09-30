package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/catalog"
	"github.com/pranavreddyg17/home-node/internal/state"
)

func signedStartupCatalog(t *testing.T, key ed25519.PrivateKey, version int64, now time.Time) []byte {
	t.Helper()
	m := catalog.Manifest{Schema: 1, Version: version, Expires: now.Add(time.Hour), Images: []catalog.Image{{ID: "files", SHA256: strings.Repeat("a", 64), Bytes: 1024, MemoryMiB: 512, VCPUs: 1, DataBytes: catalog.GiB, Protocol: 1, License: "fixture", Version: "1"}}}
	payload, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(catalog.Envelope{KeyID: catalog.KeyID(key.Public().(ed25519.PublicKey)), Payload: payload, Signature: ed25519.Sign(key, payload)})
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func TestCatalogAcceptanceCommitsOnlyTrustedProgress(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := state.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if _, err = acceptCatalog(context.Background(), store.DB, signedStartupCatalog(t, key, 3, now), pub, 4, now); err == nil {
		t.Fatal("initial rollback admitted")
	}
	var count int
	if err = store.DB.QueryRow("SELECT count(*) FROM settings WHERE key='catalog-version'").Scan(&count); err != nil || count != 0 {
		t.Fatal("failed acceptance advanced trust", count, err)
	}
	if _, err = acceptCatalog(context.Background(), store.DB, signedStartupCatalog(t, key, 5, now), pub, 4, now); err != nil {
		t.Fatal(err)
	}
	if _, err = acceptCatalog(context.Background(), store.DB, signedStartupCatalog(t, key, 4, now), pub, 4, now); err == nil {
		t.Fatal("recorded rollback admitted")
	}
	if _, err = store.DB.Exec("UPDATE settings SET value='broken' WHERE key='catalog-version'"); err != nil {
		t.Fatal(err)
	}
	if _, err = acceptCatalog(context.Background(), store.DB, signedStartupCatalog(t, key, 6, now), pub, 4, now); err == nil {
		t.Fatal("corrupt trust repaired implicitly")
	}
}
func TestConcurrentCatalogStartupKeepsHighestCommittedVersion(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	first, err := state.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := state.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	low, high := signedStartupCatalog(t, key, 4, now), signedStartupCatalog(t, key, 5, now)
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan struct {
		version int64
		err     error
	}, 2)
	for index, store := range []*state.Store{first, second} {
		wg.Add(1)
		go func(index int, store *state.Store) {
			defer wg.Done()
			<-start
			data := low
			if index == 1 {
				data = high
			}
			m, err := acceptCatalog(context.Background(), store.DB, data, pub, 4, now)
			results <- struct {
				version int64
				err     error
			}{m.Version, err}
		}(index, store)
	}
	close(start)
	wg.Wait()
	close(results)
	var highest int64
	for result := range results {
		if result.err == nil && result.version > highest {
			highest = result.version
		}
	}
	if highest == 0 {
		t.Fatal("no catalog committed")
	}
	var recorded string
	if err = first.DB.QueryRow("SELECT value FROM settings WHERE key='catalog-version'").Scan(&recorded); err != nil {
		t.Fatal(err)
	}
	floor, err := catalog.VersionFloor([]byte(recorded))
	if err != nil || floor != highest {
		t.Fatal("committed trust regressed", highest, recorded, err)
	}
}
