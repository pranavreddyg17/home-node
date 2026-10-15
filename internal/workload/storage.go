package workload

import (
	"context"
	"database/sql"
)

// FilesStorage reports logical quota accounting, not measured guest free space.
// Pending deletion remains charged until acknowledged by the guest.
type FilesStorage struct {
	QuotaBytes           int64 `json:"quotaBytes"`
	StoredBytes          int64 `json:"storedBytes"`
	TrashBytes           int64 `json:"trashBytes"`
	UploadReservedBytes  int64 `json:"uploadReservedBytes"`
	JobReservedBytes     int64 `json:"jobReservedBytes"`
	CleanupReservedBytes int64 `json:"cleanupReservedBytes"`
	ChargedBytes         int64 `json:"chargedBytes"`
	AvailableBytes       int64 `json:"availableBytes"`
	activeTransfers      int
	activeJobs           int
}

type storageQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// One statement gives an internally consistent snapshot for both the read API
// and admission inside the caller's existing transaction.
func filesStorage(ctx context.Context, q storageQuerier) (FilesStorage, error) {
	usage := FilesStorage{QuotaBytes: StorageQuota}
	err := q.QueryRowContext(ctx, `SELECT
 (SELECT coalesce(sum(size),0) FROM files),
 (SELECT coalesce(sum(size),0) FROM files WHERE trash_until IS NOT NULL),
 (SELECT coalesce(sum(size),0) FROM transfers WHERE state IN('uploading','verifying','cancelling')),
 (SELECT count(*) FROM transfers WHERE state IN('uploading','verifying','cancelling')),
 (SELECT count(*) FROM jobs WHERE state IN('queued','preparing','running','finalizing','cancelling')),
 (SELECT count(*) FROM orphan_objects WHERE workload='files') * ?`, MaxJobOutputBytes).Scan(
		&usage.StoredBytes, &usage.TrashBytes, &usage.UploadReservedBytes,
		&usage.activeTransfers, &usage.activeJobs, &usage.CleanupReservedBytes)
	if err != nil {
		return FilesStorage{}, err
	}
	usage.JobReservedBytes = int64(usage.activeJobs) * MaxJobOutputBytes
	usage.ChargedBytes = usage.StoredBytes + usage.UploadReservedBytes + usage.JobReservedBytes + usage.CleanupReservedBytes
	usage.AvailableBytes = max(0, usage.QuotaBytes-usage.ChargedBytes)
	return usage, nil
}

func (s *Service) FilesStorage(ctx context.Context) (FilesStorage, error) {
	return filesStorage(ctx, s.Store.DB)
}
