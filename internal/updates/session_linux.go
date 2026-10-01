//go:build linux

package updates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/theupdateframework/go-tuf/v2/metadata"
	"github.com/theupdateframework/go-tuf/v2/metadata/config"
	"github.com/theupdateframework/go-tuf/v2/metadata/updater"
)

// verificationSession is a serial, metadata-only TUF client. It uses the
// currently trusted root already provisioned in its protected cache, never an
// automatically substituted bootstrap root. Call Close on every path.
type verificationSession struct {
	ctx       context.Context
	cache     *lockedMetadataCache
	directory *os.File
	fetcher   *metadataFetcher
	updater   *updater.Updater
}

func newVerificationSession(ctx context.Context, provisioned *os.Root, repository string) (*verificationSession, error) {
	fetcher, err := newMetadataFetcher(ctx, repository)
	if err != nil {
		return nil, err
	}
	return newVerificationSessionWithFetcher(ctx, provisioned, fetcher)
}

func newVerificationSessionWithFetcher(ctx context.Context, provisioned *os.Root, fetcher *metadataFetcher) (*verificationSession, error) {
	cache, err := lockMetadataCache(ctx, provisioned)
	if err != nil {
		fetcher.client.CloseIdleConnections()
		return nil, err
	}
	session := &verificationSession{ctx: ctx, cache: cache, fetcher: fetcher}
	retained := false
	defer func() {
		if !retained {
			_ = session.Close()
		}
	}()
	// Validate and flush even a partial cache left by an interrupted refresh.
	// Unknown temporary entries require explicit repair, never silent adoption.
	if err = syncMetadataCacheState(ctx, cache.root, false); err != nil {
		return nil, err
	}
	rootFile, err := cache.root.Open("root.json")
	if err != nil {
		return nil, err
	}
	rootData, readErr := io.ReadAll(io.LimitReader(rootFile, maxMetadataDownload+1))
	err = errors.Join(readErr, rootFile.Close())
	if err != nil {
		return nil, err
	}
	session.directory, err = cache.root.Open(".")
	if err != nil {
		return nil, err
	}
	// The upstream library uses paths. Keep its directory descriptor open and
	// refer to that inode through procfs rather than reopening an ambient path.
	pinned := fmt.Sprintf("/proc/self/fd/%d", session.directory.Fd())
	cfg, err := config.New(fetcher.base.String(), rootData)
	if err != nil {
		return nil, err
	}
	cfg.LocalMetadataDir = pinned
	// No target downloads are exposed here; prevent EnsurePathsExist from
	// creating an unrelated target directory during metadata verification.
	cfg.LocalTargetsDir = pinned
	cfg.Fetcher = fetcher
	cfg.MaxRootRotations = 32
	cfg.MaxDelegations = 16
	cfg.RootMaxLength = 512 << 10
	cfg.TimestampMaxLength = 16 << 10
	cfg.SnapshotMaxLength = 2 << 20
	cfg.TargetsMaxLength = 5 << 20
	session.updater, err = updater.New(cfg)
	if err != nil {
		return nil, err
	}
	if err = recordUpdateClock(ctx, provisioned, session.updater.GetTrustedMetadataSet().RefTime); err != nil {
		return nil, err
	}
	refreshErr := session.updater.Refresh()
	// Rotated roots and newer timestamp evidence must survive failed refreshes
	// too. Caller cancellation must not skip this bounded persistence attempt.
	flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	flushErr := syncMetadataCacheState(flushCtx, cache.root, refreshErr == nil)
	if err = errors.Join(refreshErr, flushErr, ctx.Err()); err != nil {
		return nil, err
	}
	retained = true
	return session, nil
}

func (session *verificationSession) TargetInfo(name string) (*metadata.TargetFiles, error) {
	if err := session.ctx.Err(); err != nil {
		return nil, err
	}
	target, lookupErr := session.updater.GetTargetInfo(name)
	if target == nil && lookupErr == nil {
		lookupErr = errDownloadPolicy
	}
	flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := errors.Join(lookupErr, syncMetadataCache(flushCtx, session.cache.root), session.ctx.Err()); err != nil {
		return nil, err
	}
	// Return independent evidence; callers cannot mutate the trusted metadata set.
	data, err := json.Marshal(target)
	if err != nil {
		return nil, err
	}
	var result metadata.TargetFiles
	if err = json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	result.Path = target.Path
	return &result, nil
}

func (session *verificationSession) Close() error {
	session.fetcher.client.CloseIdleConnections()
	var directoryErr error
	if session.directory != nil {
		directoryErr = session.directory.Close()
	}
	return errors.Join(directoryErr, session.cache.Close())
}
