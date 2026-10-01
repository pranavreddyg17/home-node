//go:build linux

package updates

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/theupdateframework/go-tuf/v2/metadata"
	"golang.org/x/sys/unix"
)

const maxPackageBytes int64 = 512 << 20
const packageDiskReserve uint64 = 2 << 30

var packagePath = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]{0,191}\.deb$`)

// AcquirePackage returns a read-only verified package descriptor, never install
// authority. targetsURL and staging are trusted service configuration. The
// session's exclusive lock must remain held throughout acquisition. Partial
// files remain explicit failed intent and are never used as verified packages.
func (session *verificationSession) AcquirePackage(name, targetsURL string, staging *os.Root, policy ReleasePolicy) (*os.File, error) {
	target, err := session.TargetInfo(name)
	if err != nil {
		return nil, err
	}
	release, err := parseReleaseMetadata(target, policy)
	if err != nil {
		return nil, err
	}
	for _, evidenceName := range []string{release.SBOMTarget, release.ProvenanceTarget} {
		evidence, err := session.TargetInfo(evidenceName)
		if err != nil {
			return nil, err
		}
		if evidence.Length < 1 || evidence.Length > 8<<20 || len(evidence.Hashes["sha256"]) != sha256.Size {
			return nil, errReleasePolicy
		}
	}
	fetcher, err := newMetadataFetcher(session.ctx, targetsURL)
	if err != nil {
		return nil, err
	}
	defer fetcher.client.CloseIdleConnections()
	if fetcher.base.Host != session.fetcher.base.Host {
		return nil, errDownloadPolicy
	}
	fetcher.client.Timeout = 10 * time.Minute
	consistent := session.updater.GetTrustedMetadataSet().Root.Signed.ConsistentSnapshot
	return acquireVerifiedPackage(session.ctx, fetcher, target, consistent, staging)
}

func acquireVerifiedPackage(ctx context.Context, fetcher *metadataFetcher, target *metadata.TargetFiles, consistent bool, staging *os.Root) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if target == nil || staging == nil || !packagePath.MatchString(target.Path) || path.Clean(target.Path) != target.Path || strings.Contains(target.Path, "//") || target.Length < 1 || target.Length > maxPackageBytes || len(target.Hashes["sha256"]) != sha256.Size {
		return nil, errDownloadPolicy
	}
	hashes := map[string]hash.Hash{"sha256": sha256.New()}
	for algorithm, expected := range target.Hashes {
		switch algorithm {
		case "sha256":
		case "sha512":
			if len(expected) != sha512.Size {
				return nil, errDownloadPolicy
			}
			hashes[algorithm] = sha512.New()
		default:
			return nil, errDownloadPolicy
		}
	}
	digest := hex.EncodeToString(target.Hashes["sha256"])
	remote := target.Path
	if consistent {
		remote = path.Join(path.Dir(remote), digest+"."+path.Base(remote))
	}
	location := fetcher.base.String() + remote
	directory, err := staging.Open(".")
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	info, err := directory.Stat()
	var native unix.Stat_t
	var space unix.Statfs_t
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 || unix.Fstat(int(directory.Fd()), &native) != nil || native.Uid != uint32(os.Geteuid()) || unix.Fstatfs(int(directory.Fd()), &space) != nil || space.Bsize <= 0 {
		return nil, errDownloadPolicy
	}
	block := uint64(space.Bsize)
	required := uint64(target.Length) + packageDiskReserve
	if space.Bavail < (required+block-1)/block {
		return nil, errDownloadPolicy
	}
	verifiedName := digest + ".deb"
	if _, err = staging.Lstat(verifiedName); !os.IsNotExist(err) {
		return nil, errDownloadPolicy
	}
	operation, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	request, err := http.NewRequestWithContext(operation, http.MethodGet, location, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept-Encoding", "identity")
	request.Header.Set("User-Agent", "HomeNode-update")
	response, err := fetcher.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, &metadata.ErrDownloadHTTP{StatusCode: response.StatusCode, URL: location}
	}
	if response.ContentLength >= 0 && response.ContentLength != target.Length || response.Header.Get("Content-Encoding") != "" && response.Header.Get("Content-Encoding") != "identity" {
		return nil, errDownloadPolicy
	}
	pendingName := digest + ".pending"
	file, err := staging.OpenFile(pendingName, os.O_CREATE|os.O_EXCL|os.O_WRONLY|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()
	writers := []io.Writer{file}
	for _, hash := range hashes {
		writers = append(writers, hash)
	}
	count, err := io.Copy(io.MultiWriter(writers...), io.LimitReader(response.Body, target.Length+1))
	if err != nil {
		return nil, err
	}
	if count != target.Length {
		return nil, errDownloadPolicy
	}
	for algorithm, hash := range hashes {
		if !bytes.Equal(hash.Sum(nil), target.Hashes[algorithm]) {
			return nil, errDownloadPolicy
		}
	}
	if err = operation.Err(); err != nil {
		return nil, err
	}
	if err = file.Chmod(0400); err != nil {
		return nil, err
	}
	before, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if err = errors.Join(file.Sync(), file.Close()); err != nil {
		closed = true
		return nil, err
	}
	closed = true
	// Publish without replacing a previously verified or foreign file.
	if err = unix.Renameat2(int(directory.Fd()), pendingName, int(directory.Fd()), verifiedName, unix.RENAME_NOREPLACE); err != nil {
		return nil, err
	}
	if err = directory.Sync(); err != nil {
		return nil, err
	}
	verified, err := staging.OpenFile(verifiedName, os.O_RDONLY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	after, err := verified.Stat()
	if err != nil || !os.SameFile(before, after) {
		verified.Close()
		return nil, errDownloadPolicy
	}
	if err = operation.Err(); err != nil {
		verified.Close()
		return nil, err
	}
	return verified, nil
}
