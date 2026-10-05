package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/pranavreddyg17/home-node/internal/state"
)

type recoveryManagementOutput struct {
	PlanSHA256 string `json:"planSha256"`
	SHA256     string `json:"sha256"`
	Bytes      int64  `json:"bytes"`
}

// inspectReboundRecoveryManagement identifies a validated rebound private
// database for later durable output recording/publication. Exclusive root
// ownership must continue; the returned value is not itself a durable receipt.
func inspectReboundRecoveryManagement(ctx context.Context, root *os.Root) (output recoveryManagementOutput, result error) {
	plan, err := loadRecoveryInstallPlan(ctx, root)
	if err != nil {
		return output, err
	}
	const stage = ".recovery-management.stage"
	expected, err := root.Lstat(stage)
	if err != nil || !expected.Mode().IsRegular() || expected.Mode().Perm() != 0600 || expected.Size() <= 0 || expected.Size() > 256<<20 {
		return output, ErrManifest
	}
	file, err := root.Open(stage)
	if err != nil {
		return output, err
	}
	defer func() {
		result = errors.Join(result, file.Close())
		if result != nil {
			output = recoveryManagementOutput{}
		}
	}()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(expected, opened) {
		return output, ErrManifest
	}
	metadata, err := state.ReadRecoveryMetadata(ctx, file)
	if err != nil {
		return output, err
	}
	if metadata.OwnerID != plan.OwnerID || metadata.Epoch != plan.RecoveryEpoch || len(metadata.Apps) != len(plan.Disks) {
		return output, ErrManifest
	}
	bindings := map[string]string{}
	for _, disk := range plan.Disks {
		bindings[disk.Workload] = disk.InstanceID
	}
	for _, app := range metadata.Apps {
		if bindings[app.Workload] != app.InstanceID {
			return output, ErrManifest
		}
	}
	hash := sha256.New()
	remaining := opened.Size()
	buffer := make([]byte, 1<<20)
	for remaining > 0 {
		if err = ctx.Err(); err != nil {
			return output, err
		}
		n, readErr := io.ReadFull(file, buffer[:min(int64(len(buffer)), remaining)])
		if readErr != nil {
			return output, ErrManifest
		}
		hash.Write(buffer[:n])
		remaining -= int64(n)
	}
	var extra [1]byte
	if n, readErr := file.Read(extra[:]); n != 0 || readErr != io.EOF {
		return output, ErrManifest
	}
	current, err := root.Lstat(stage)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(opened, current) || current.Size() != opened.Size() || current.Mode().Perm() != 0600 {
		return output, ErrManifest
	}
	data, err := json.Marshal(plan)
	if err != nil {
		return output, err
	}
	planHash := sha256.Sum256(data)
	if err = ctx.Err(); err != nil {
		return output, err
	}
	return recoveryManagementOutput{PlanSHA256: hex.EncodeToString(planHash[:]), SHA256: hex.EncodeToString(hash.Sum(nil)), Bytes: opened.Size()}, nil
}
