package install

import "reflect"

// gatewayTLSRetirementState admits the acknowledged permission phase or exactly
// one unacknowledged mutation. Native callers must independently pin inodes,
// verify contents and ACLs, and retain activation exclusion. This function
// grants no mutation authority and never infers progress from a changed hash.
func gatewayTLSRetirementState(plan GatewayTLSRetirementPlan, completed int, ready bool, observed []ConfigurationRemovalFile) (int, error) {
	if completed < 0 || completed > len(plan.Changes) || (ready && completed != len(plan.Changes)) || len(plan.Changes) != 5 || len(observed) != 3 {
		return 0, ErrConflict
	}
	// Regenerate the fixed plan rather than trusting caller-supplied operations.
	source := journal{ID: plan.InstallationID, Digest: plan.InstallationDigest, Phase: "installed", Items: []record{{Path: "etc/homenode/tls", Directory: true, UID: 0, GID: plan.Identity.GID, Mode: 0750, State: "created"}}}
	canonical, err := gatewayTLSRetirementPlan(source, plan.Identity, plan.Key, plan.Certificate)
	if err != nil || !reflect.DeepEqual(canonical, plan) {
		return 0, ErrConflict
	}
	expected := []ConfigurationRemovalFile{plan.Key, plan.Certificate, {Path: "etc/homenode/tls", UID: 0, GID: plan.Identity.GID, Mode: 0750}}
	phase := -1
	for step := 0; step <= len(plan.Changes); step++ {
		if reflect.DeepEqual(expected, observed) && step >= completed && step <= completed+1 && (!ready || step == completed) {
			phase = step
		}
		if step == len(plan.Changes) {
			break
		}
		change := plan.Changes[step]
		for index := range expected {
			if expected[index].Path == change.Path {
				if expected[index].Mode != change.BeforeMode || expected[index].GID != change.BeforeGID {
					return 0, ErrConflict
				}
				expected[index].Mode, expected[index].GID = change.AfterMode, change.AfterGID
			}
		}
	}
	// Choose the furthest admissible identical phase for a no-op certificate GID change.
	// It can advance only the immediately pending operation, never skip authority.
	if phase < completed || phase > completed+1 || (ready && phase != completed) {
		return 0, ErrConflict
	}
	return phase, nil
}
