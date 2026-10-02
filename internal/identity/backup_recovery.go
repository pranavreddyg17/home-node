package identity

import (
	"context"
	"database/sql"
	"regexp"
	"slices"

	"github.com/pranavreddyg17/home-node/internal/state"
)

var backupRecoveryJobID = regexp.MustCompile(`^[a-zA-Z0-9_-]{20,64}$`)

func BackupRecoveryApprovalResources(body []byte, jobID, requestKey string) ([]string, error) {
	if _, err := approvalObject(body, nil); err != nil || !backupRecoveryJobID.MatchString(jobID) || len(requestKey) < 16 || len(requestKey) > 128 {
		return nil, ErrDenied
	}
	resources := []string{jobID, state.Hash(requestKey)}
	slices.Sort(resources)
	return resources, nil
}

// AuthorizeBackupResumeApproved consumes fresh approval only with qualified
// released-job ownership. This does not restore data or release runtime authority.
func (s *Service) AuthorizeBackupResumeApproved(ctx context.Context, actor Session, grant string, body []byte, jobID, requestKey string, policy int64) (string, state.MaintenanceJob, error) {
	resources, err := BackupRecoveryApprovalResources(body, jobID, requestKey)
	if err != nil {
		return "", state.MaintenanceJob{}, err
	}
	var token string
	var job state.MaintenanceJob
	err = s.ConsumeApproval(ctx, actor, grant, "backup.resume", resources, body, policy, func(tx *sql.Tx) error {
		var err error
		token, job, err = state.AuthorizeReleasedBackupRecoveryTx(tx, actor.Device.ID, jobID)
		return err
	})
	if err != nil {
		return "", state.MaintenanceJob{}, err
	}
	return token, job, nil
}
