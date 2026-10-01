//go:build linux

package updates

import (
	"context"
	"errors"
	"os"
	"path"
	"strings"
)

// AcquireRelease refreshes protected TUF metadata and acquires one signed
// release while retaining exclusive cache ownership. Repository locations and
// policy must come from protected host configuration, never a browser request.
// The returned package is read-only; acquisition grants no install authority.
// Caller owns the returned descriptor and must close it. Provisioned and staging
// remain caller-owned and are not closed by this operation.
func AcquireRelease(ctx context.Context, provisioned, staging *os.Root, metadataURL, targetsURL, target string, policy ReleasePolicy) (result *AcquiredRelease, resultErr error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if provisioned == nil || staging == nil || policy.MinimumSequence < 1 || policy.MinimumCatalogVersion < 1 || policy.CurrentStateSchema < 1 || policy.CurrentStateSchema > 1024 || !packagePath.MatchString(target) || path.Clean(target) != target || strings.Contains(target, "//") {
		return nil, errReleasePolicy
	}
	// Validate both repository scopes before refreshing or writing metadata.
	metadataFetcher, err := newMetadataFetcher(ctx, metadataURL)
	if err != nil {
		return nil, err
	}
	defer metadataFetcher.client.CloseIdleConnections()
	targetFetcher, err := newMetadataFetcher(ctx, targetsURL)
	if err != nil {
		return nil, err
	}
	defer targetFetcher.client.CloseIdleConnections()
	if metadataFetcher.base.Host != targetFetcher.base.Host {
		return nil, errDownloadPolicy
	}
	return acquireReleaseWithFetchers(ctx, provisioned, staging, target, policy, metadataFetcher, targetFetcher)
}

// The private transport seam permits signed TLS repository fixtures without
// exposing alternate trust stores or network clients to production callers.
func acquireReleaseWithFetchers(ctx context.Context, provisioned, staging *os.Root, target string, policy ReleasePolicy, metadataFetcher, targetFetcher *metadataFetcher) (result *AcquiredRelease, resultErr error) {
	session, err := newVerificationSessionWithFetcher(ctx, provisioned, metadataFetcher)
	if err != nil {
		return nil, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, session.Close(), ctx.Err())
		if resultErr != nil && result != nil {
			resultErr = errors.Join(resultErr, result.Close())
			result = nil
		}
	}()
	return session.acquirePackageWithFetcher(target, targetFetcher, staging, policy)
}
