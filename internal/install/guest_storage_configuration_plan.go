package install

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

// Plan both configuration records as one transition; neither file is published.
func planGuestStorageConfiguration(ctx context.Context, installed journal, sourcePolicy, sourceEnv []byte, plan GuestStorageProvisioningPlan) (journal, []byte, []byte, error) {
	proposed, policyBytes, err := planGuestStorageRuntimePolicy(ctx, installed, sourcePolicy, plan)
	if err != nil {
		return journal{}, nil, nil, err
	}
	var policy supervisor.Policy
	if json.Unmarshal(policyBytes, &policy) != nil || policy.Generation < 2 {
		return journal{}, nil, nil, ErrConflict
	}
	keys := []string{"TAILNET_IP", "HTTPS_PORT", "HTTPS_ORIGIN", "POLICY_GENERATION", "CONTROLLER_UID", "RUNTIME_GID", "TRANSFER_GID"}
	if len(sourceEnv) == 0 || len(sourceEnv) > 16384 || bytes.IndexByte(sourceEnv, 0) >= 0 {
		return journal{}, nil, nil, ErrConflict
	}
	lines := strings.Split(string(sourceEnv), "\n")
	if len(lines) != len(keys)+1 || lines[len(keys)] != "" {
		return journal{}, nil, nil, ErrConflict
	}
	values := make([]string, len(keys))
	for i, key := range keys {
		value, ok := strings.CutPrefix(lines[i], key+"=")
		if !ok || value == "" || strings.ContainsAny(value, "\r") {
			return journal{}, nil, nil, ErrConflict
		}
		values[i] = value
	}
	if values[3] != strconv.FormatInt(policy.Generation-1, 10) || values[4] != strconv.FormatUint(uint64(policy.ControllerUID), 10) {
		return journal{}, nil, nil, ErrConflict
	}
	for _, value := range values[5:] {
		gid, err := strconv.ParseUint(value, 10, 31)
		if err != nil || gid == 0 || uint32(gid) == plan.GuestGID || strconv.FormatUint(gid, 10) != value {
			return journal{}, nil, nil, ErrConflict
		}
	}
	lines[3] = keys[3] + "=" + strconv.FormatInt(policy.Generation, 10)
	envBytes := []byte(strings.Join(lines, "\n"))
	matched := 0
	for i, item := range proposed.Items {
		if item.Path != "etc/homenode/services.env" {
			continue
		}
		matched++
		if item.Directory || item.Mode != 0644 || item.UID != 0 || item.GID != 0 || item.SHA256 != digest(sourceEnv) || item.State != "created" && item.State != "existing" {
			return journal{}, nil, nil, ErrConflict
		}
		proposed.Items[i].SHA256 = digest(envBytes)
	}
	if matched != 1 {
		return journal{}, nil, nil, ErrConflict
	}
	normalized := append([]record(nil), proposed.Items...)
	for i := range normalized {
		normalized[i].State = "pending"
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return journal{}, nil, nil, err
	}
	proposed.Digest = digest(encoded)
	if err := ctx.Err(); err != nil {
		return journal{}, nil, nil, err
	}
	return proposed, policyBytes, envBytes, nil
}
