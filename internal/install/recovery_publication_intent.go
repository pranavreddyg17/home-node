package install

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// Identity is bound to verified staging by the transaction before recording.
// This value alone grants no content, directory or runtime authority.
type recoveryPublicationIdentity struct {
	Device uint64 `json:"device"`
	Inode  uint64 `json:"inode"`
	Bytes  int64  `json:"bytes"`
	UID    uint32 `json:"uid"`
	GID    uint32 `json:"gid"`
}

type recoveryPublicationIntent struct {
	Version              int                         `json:"version"`
	ConfigurationID      string                      `json:"configurationId"`
	ConfigurationDigest  string                      `json:"configurationDigest"`
	RecoveryIntentSHA256 string                      `json:"recoveryIntentSha256"`
	FileName             string                      `json:"fileName"`
	ContentSHA256        string                      `json:"contentSha256"`
	Identity             recoveryPublicationIdentity `json:"identity"`
}

func recoveryPublicationRecordName(final string) (string, error) {
	if final == "management.db" {
		return "recovery-management-publication.json", nil
	}
	id := strings.TrimSuffix(final, ".raw")
	if !guestproto.ValidID(id) || final != id+".raw" {
		return "", ErrPlan
	}
	return "recovery-" + id + "-publication.json", nil
}

func validRecoveryRecordName(name string) bool {
	if name == "recovery.json" || name == "recovery-staged.json" || name == "recovery-management-publication.json" {
		return true
	}
	id := strings.TrimSuffix(strings.TrimPrefix(name, "recovery-"), "-publication.json")
	return guestproto.ValidID(id) && name == "recovery-"+id+"-publication.json"
}

func validateRecoveryPublicationIntent(intent recoveryPublicationIntent) error {
	canonicalHex := func(value string, size int) bool {
		decoded, err := hex.DecodeString(value)
		return err == nil && len(decoded) == size && hex.EncodeToString(decoded) == value
	}
	if intent.Version != 1 || !canonicalHex(intent.ConfigurationID, 16) || !canonicalHex(intent.ConfigurationDigest, 32) || !canonicalHex(intent.RecoveryIntentSHA256, 32) || !canonicalHex(intent.ContentSHA256, 32) {
		return ErrPlan
	}
	if _, err := recoveryPublicationRecordName(intent.FileName); err != nil {
		return err
	}
	i := intent.Identity
	if i.Inode == 0 || i.Bytes <= 0 || i.Bytes > 512<<30 || i.UID == 0 || i.UID > 1<<31-1 || i.GID == 0 || i.GID > 1<<31-1 {
		return ErrPlan
	}
	return nil
}

// Caller must independently qualify installation, recovery selection, content,
// ownership intent and retained exclusion. Recording never publishes a file.
func (e *Engine) commitRecoveryPublicationIntent(ctx context.Context, intent recoveryPublicationIntent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateRecoveryPublicationIntent(intent); err != nil {
		return err
	}
	name, _ := recoveryPublicationRecordName(intent.FileName)
	data, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	return e.commitRecoveryRecord(ctx, name, data)
}

// withRecoveryPublicationIntent requires an independently qualified recovery
// selection and keeps the exact publication record open across the consumer.
// The caller still supplies live account, storage and runtime exclusion proof.
func (e *Engine) withRecoveryPublicationIntent(ctx context.Context, intent recoveryPublicationIntent, recovery recoveryIntent, use func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if use == nil || validateRecoveryPublicationIntent(intent) != nil || recovery.Version != 1 {
		return ErrPlan
	}
	encodedRecovery, err := json.Marshal(recovery)
	if err != nil {
		return err
	}
	if intent.ConfigurationID != recovery.ConfigurationID || intent.ConfigurationDigest != recovery.ConfigurationDigest || intent.RecoveryIntentSHA256 != digest(encodedRecovery) {
		return ErrConflict
	}
	matched := intent.FileName == "management.db" && intent.ContentSHA256 == recovery.Recovery.ManagementSHA256 && intent.Identity.Bytes == recovery.Recovery.ManagementBytes
	for _, disk := range recovery.Recovery.Disks {
		if intent.FileName == disk.InstanceID+".raw" && intent.ContentSHA256 == disk.SourceSHA256 && intent.Identity.Bytes == disk.Bytes {
			matched = true
		}
	}
	if !matched {
		return ErrConflict
	}
	name, _ := recoveryPublicationRecordName(intent.FileName)
	data, err := json.Marshal(intent)
	if err != nil {
		return err
	}
	return e.withRecoveryRecord(ctx, "recovery.json", encodedRecovery, func(ctx context.Context) error {
		return e.withRecoveryRecord(ctx, name, data, use)
	})
}
