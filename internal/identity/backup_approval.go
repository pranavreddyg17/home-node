package identity

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"

	"github.com/pranavreddyg17/home-node/internal/state"
)

// BackupApprovalResources binds an exact request to the registered repository
// identity and request key. The caller supplies the trusted registered identity;
// neither a filesystem path nor a repository credential belongs in this body.
func BackupApprovalResources(body []byte, registeredRepository, requestKey string) ([]string, error) {
	object, err := approvalObject(body, []string{"repositoryId"})
	if err != nil || len(requestKey) < 16 || len(requestKey) > 128 {
		return nil, ErrDenied
	}
	var repository string
	if json.Unmarshal(object["repositoryId"], &repository) != nil || repository != registeredRepository {
		return nil, ErrDenied
	}
	digest, err := hex.DecodeString(repository)
	if err != nil || len(digest) != 32 || hex.EncodeToString(digest) != repository {
		return nil, ErrDenied
	}
	return []string{repository, state.Hash(requestKey)}, nil
}

// AdmitBackupApproved consumes approval and records maintenance admission in
// one transaction. Returned authority stays inside the controller coordinator;
// this method does not dispatch a worker or authorize a filesystem destination.
func (s *Service) AdmitBackupApproved(ctx context.Context, actor Session, grant string, body []byte, repository, requestKey string, policy int64) (string, state.MaintenanceJob, error) {
	resources, err := BackupApprovalResources(body, repository, requestKey)
	if err != nil {
		return "", state.MaintenanceJob{}, err
	}
	var token string
	var job state.MaintenanceJob
	err = s.ConsumeApproval(ctx, actor, grant, "backup.create", resources, body, policy, func(tx *sql.Tx) error {
		var err error
		token, job, err = state.BeginMaintenanceJobTx(tx, actor.Device.ID)
		return err
	})
	if err != nil {
		return "", state.MaintenanceJob{}, err
	}
	return token, job, nil
}
