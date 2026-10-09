package install

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

// Plan one authenticated configuration transition. Publication must coordinate
// services.env's generation and retain storage/runtime exclusion separately.
func planGuestStorageRuntimePolicy(ctx context.Context, installed journal, source []byte, plan GuestStorageProvisioningPlan) (journal, []byte, error) {
	if err := ctx.Err(); err != nil {
		return journal{}, nil, err
	}
	if installed.Version != 1 || installed.Phase != "installed" || len(source) == 0 || len(source) > 16384 {
		return journal{}, nil, ErrPlan
	}
	if _, err := canonicalGuestStoragePlan(ctx, plan); err != nil {
		return journal{}, nil, err
	}
	var policy supervisor.Policy
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&policy) != nil || decoder.Decode(new(any)) != io.EOF || policy.Validate() != nil || policy.GuestIdentity != nil || policy.Generation == math.MaxInt64 {
		return journal{}, nil, ErrConflict
	}
	for _, uid := range []uint32{policy.ControllerUID, policy.TransferUID} {
		found := false
		for _, service := range plan.Identity.ServiceUIDs {
			if service == uid {
				found = true
			}
		}
		if !found {
			return journal{}, nil, ErrConflict
		}
	}
	policy.Generation++
	policy.GuestIdentity = &supervisor.ReservedGuestPolicy{Version: 1, FirstUID: plan.Identity.First, LastUID: plan.Identity.Last, GuestGID: plan.GuestGID}
	if err := policy.Validate(); err != nil {
		return journal{}, nil, err
	}
	desired, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return journal{}, nil, err
	}
	desired = append(desired, '\n')
	normalized := append([]record(nil), installed.Items...)
	proposed := installed
	proposed.Items = append([]record(nil), installed.Items...)
	matched := 0
	for i, item := range installed.Items {
		normalized[i].State = "pending"
		if item.Path != "etc/homenode/runtime-policy.json" {
			continue
		}
		matched++
		if item.Directory || item.Mode != 0600 || item.UID != 0 || item.GID != 0 || item.SHA256 != digest(source) || item.State != "created" && item.State != "existing" {
			return journal{}, nil, ErrConflict
		}
		proposed.Items[i].SHA256 = digest(desired)
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return journal{}, nil, err
	}
	if matched != 1 || digest(encoded) != installed.Digest {
		return journal{}, nil, ErrConflict
	}
	for i, item := range proposed.Items {
		normalized[i] = item
		normalized[i].State = "pending"
	}
	encoded, err = json.Marshal(normalized)
	if err != nil {
		return journal{}, nil, err
	}
	proposed.Digest = digest(encoded)
	if err := ctx.Err(); err != nil {
		return journal{}, nil, err
	}
	return proposed, desired, nil
}
