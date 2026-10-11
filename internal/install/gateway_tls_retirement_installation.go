package install

import (
	"encoding/json"
	"reflect"
)

// This pure transformation grants no publication authority. The caller must
// verify Ready against retained TLS inodes and a live exclusion guard before
// saving. Only the retained TLS directory permission record changes; its source
// digest remains independently reconstructible from the retirement intent.
func gatewayTLSRetiredInstallation(source journal, intent gatewayTLSRetirementIntent) (journal, error) {
	if intent.Version != 1 || !intent.Ready || intent.Completed != 5 || source.ID != intent.Plan.InstallationID || source.Digest != intent.Plan.InstallationDigest {
		return journal{}, ErrConflict
	}
	if err := validateTLSInstallationSource(source); err != nil {
		return journal{}, err
	}
	plan, err := gatewayTLSRetirementPlan(source, intent.Plan.Identity, intent.Plan.Key, intent.Plan.Certificate)
	if err != nil || !reflect.DeepEqual(plan, intent.Plan) {
		return journal{}, ErrConflict
	}
	next := source
	next.Items = append([]record(nil), source.Items...)
	for index := range next.Items {
		if next.Items[index].Path == "etc/homenode/tls" {
			next.Items[index].GID = 0
			next.Items[index].Mode = 0700
		}
	}
	next.Digest = normalizedTLSInstallationDigest(next.Items)
	if len(gatewayRemovalIdentityReferences(next, intent.Plan.Identity)) != 0 {
		return journal{}, ErrConflict
	}
	return next, nil
}

// Reconstruct the immutable pre-retirement source after publication. Any change
// beyond the single planned TLS directory transition changes the source digest
// and is refused, including otherwise valid changes to retained data records.
func gatewayTLSSourceInstallation(retired journal, intent gatewayTLSRetirementIntent) (journal, error) {
	if err := validateTLSInstallationSource(retired); err != nil {
		return journal{}, err
	}
	source := retired
	source.Items = append([]record(nil), retired.Items...)
	found := false
	for index := range source.Items {
		if source.Items[index].Path == "etc/homenode/tls" {
			if source.Items[index].GID != 0 || source.Items[index].Mode != 0700 {
				return journal{}, ErrConflict
			}
			source.Items[index].GID = intent.Plan.Identity.GID
			source.Items[index].Mode = 0750
			found = true
		}
	}
	if !found {
		return journal{}, ErrConflict
	}
	source.Digest = normalizedTLSInstallationDigest(source.Items)
	expected, err := gatewayTLSRetiredInstallation(source, intent)
	if err != nil || !reflect.DeepEqual(expected, retired) {
		return journal{}, ErrConflict
	}
	return source, nil
}
func normalizedTLSInstallationDigest(items []record) string {
	normalized := append([]record(nil), items...)
	for index := range normalized {
		normalized[index].State = "pending"
	}
	encoded, _ := json.Marshal(normalized)
	return digest(encoded)
}
func validateTLSInstallationSource(source journal) error {
	if source.Version != 1 || source.Phase != "installed" || len(source.Items) == 0 || len(source.Items) > maxInstallationItems || normalizedTLSInstallationDigest(source.Items) != source.Digest {
		return ErrConflict
	}
	seen := map[string]bool{}
	for _, item := range source.Items {
		if !validRecord(item, 0) || seen[item.Path] || (item.State != "created" && item.State != "existing") || (!item.Directory && item.State == "existing") {
			return ErrConflict
		}
		seen[item.Path] = true
	}
	return nil
}

// Accept either the original source or its single verified retirement successor.
func gatewayTLSInstallationSource(current journal, intent gatewayTLSRetirementIntent) (journal, error) {
	if current.ID != intent.Plan.InstallationID {
		return journal{}, ErrConflict
	}
	if current.Digest == intent.Plan.InstallationDigest {
		if err := validateTLSInstallationSource(current); err != nil {
			return journal{}, err
		}
		plan, err := gatewayTLSRetirementPlan(current, intent.Plan.Identity, intent.Plan.Key, intent.Plan.Certificate)
		if err != nil || !reflect.DeepEqual(plan, intent.Plan) {
			return journal{}, ErrConflict
		}
		return current, nil
	}
	return gatewayTLSSourceInstallation(current, intent)
}
