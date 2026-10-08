//go:build linux

package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"syscall"

	"github.com/pranavreddyg17/home-node/internal/backup"
	"golang.org/x/sys/unix"
)

// prepareRecoveryPublicationFile creates fresh staging in an independently
// qualified destination. Caller holds installation and activation exclusion,
// a verified source scope and independently qualified target ownership.
// Existing/partial staging is preserved for journal-based reconciliation.
func (e *Engine) prepareRecoveryPublicationFile(ctx context.Context, destination *os.Root, source backup.PreparedRecoveryFile, recovery recoveryIntent, uid, gid uint32, guard func(context.Context) error) (intent recoveryPublicationIntent, result error) {
	if err := ctx.Err(); err != nil {
		return intent, err
	}
	if destination == nil || source.File == nil || guard == nil || uid == 0 || uid > 1<<31-1 || gid == 0 || gid > 1<<31-1 || recovery.Version != 1 {
		return intent, ErrPlan
	}
	if _, err := recoveryPublicationRecordName(source.Name); err != nil {
		return intent, err
	}
	encoded, err := json.Marshal(recovery)
	if err != nil {
		return intent, err
	}
	intent = recoveryPublicationIntent{Version: 1, ConfigurationID: recovery.ConfigurationID, ConfigurationDigest: recovery.ConfigurationDigest, RecoveryIntentSHA256: digest(encoded), FileName: source.Name, ContentSHA256: source.SHA256, Identity: recoveryPublicationIdentity{Inode: 1, Bytes: source.Bytes, UID: uid, GID: gid}}
	if err := validateRecoveryPublicationIntent(intent); err != nil {
		return recoveryPublicationIntent{}, err
	}
	matched := source.Name == "management.db" && source.Bytes == recovery.Recovery.ManagementBytes && source.SHA256 == recovery.Recovery.ManagementSHA256
	for _, disk := range recovery.Recovery.Disks {
		if source.Name == disk.InstanceID+".raw" && source.Bytes == disk.Bytes && source.SHA256 == disk.SourceSHA256 {
			matched = true
		}
	}
	if !matched {
		return recoveryPublicationIntent{}, ErrConflict
	}
	if err := e.matchRecoveryRecord(ctx, "recovery.json", encoded); err != nil {
		return recoveryPublicationIntent{}, err
	}
	if err := guard(ctx); err != nil {
		return recoveryPublicationIntent{}, err
	}
	before, err := source.File.Stat()
	if err != nil || !accountJournalFileAdmitted(before, e.owner, source.Bytes) || before.Size() != source.Bytes {
		return recoveryPublicationIntent{}, ErrConflict
	}
	stage := ".recovery-management.publish"
	if source.Name != "management.db" {
		stage = ".recovery-" + source.Name[:len(source.Name)-4] + ".publish"
	}
	out, err := destination.OpenFile(stage, os.O_CREATE|os.O_EXCL|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return recoveryPublicationIntent{}, err
	}
	defer func() {
		result = errors.Join(result, out.Close())
		if result != nil {
			intent = recoveryPublicationIntent{}
		}
	}()
	hash := sha256.New()
	writer := &recoveryCopyWriter{file: out, remaining: source.Bytes}
	n, err := io.Copy(io.MultiWriter(writer, hash), contextReader{ctx, io.NewSectionReader(source.File, 0, source.Bytes)})
	if err != nil {
		return intent, err
	}
	after, err := source.File.Stat()
	if err != nil || !os.SameFile(before, after) || !accountJournalFileAdmitted(after, e.owner, source.Bytes) || after.Size() != source.Bytes || n != source.Bytes || hex.EncodeToString(hash.Sum(nil)) != source.SHA256 {
		return intent, ErrConflict
	}
	var stat unix.Stat_t
	if unix.Fstat(int(out.Fd()), &stat) != nil || stat.Mode != unix.S_IFREG|0600 || stat.Nlink != 1 || int(stat.Uid) != e.owner || stat.Size != source.Bytes {
		return intent, ErrConflict
	}
	intent.Identity.Device, intent.Identity.Inode = uint64(stat.Dev), stat.Ino
	if err := out.Sync(); err != nil {
		return intent, err
	}
	if err := syncDirectory(destination, "."); err != nil {
		return intent, err
	}
	if err := guard(ctx); err != nil {
		return intent, err
	}
	// The durable ownership intent precedes descriptor-based chown.
	if err := e.commitRecoveryPublicationIntent(ctx, intent); err != nil {
		return intent, err
	}
	if err := guard(ctx); err != nil {
		return intent, err
	}
	if err := out.Chown(int(uid), int(gid)); err != nil {
		return intent, err
	}
	if err := verifyRecoveryPublicationDescriptor(ctx, out, intent); err != nil {
		return intent, err
	}
	current, err := destination.Lstat(stage)
	opened, statErr := out.Stat()
	if err != nil || statErr != nil || !os.SameFile(current, opened) {
		return intent, ErrConflict
	}
	if err := guard(ctx); err != nil {
		return intent, err
	}
	return intent, ctx.Err()
}
