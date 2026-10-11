package install

import "strings"

// validateRemovalDormantUnit admits only the owned uninstall transition. Missing
// is supplied from canonical intent and filesystem observation, never from the
// manager response alone. Ordinary recovery continues requiring loaded units
// with no pending reload.
func validateRemovalDormantUnit(data []byte, unit string, missing bool) error {
	switch unit {
	case "homenode-gateway.service", "homenode-control.service", "homenode-transfer.service", "homenode-supervisor.service", "homenode-backup.service", "homenode-backup-credential.socket", "homenode-app-maintenance.socket", "homenode-inspect.service", "homenode.slice":
	default:
		return ErrPlan
	}
	common := map[string]string{"Job": "", "Id": unit, "DropInPaths": "", "Transient": "no", "ActiveState": "inactive", "SubState": "dead", "MainPID": "0", "ControlPID": "0"}
	if !strings.HasSuffix(unit, ".service") {
		delete(common, "MainPID")
		delete(common, "ControlPID")
	}
	// The running manager may retain the previously loaded, now absent fragment
	// until daemon-reload. Its path must remain exactly the owned unit path.
	common["LoadState"] = "loaded"
	common["FragmentPath"] = "/etc/systemd/system/" + unit
	reloadStates := []string{"no"}
	if missing {
		reloadStates = append(reloadStates, "yes")
	}
	for _, reload := range reloadStates {
		common["NeedDaemonReload"] = reload
		if validateRecoveryProperties(data, common) == nil {
			return nil
		}
	}
	if !missing {
		return ErrConflict
	}
	// After reload (or manager collection), only a genuinely absent, inactive
	// unit is accepted. A masked/failed/generated/aliased unit is not absence.
	common["LoadState"] = "not-found"
	common["FragmentPath"] = ""
	common["NeedDaemonReload"] = "no"
	// Slice files are optional in systemd: a collected slice is synthesized as
	// loaded with no fragment. Filesystem absence is independently established
	// by the caller; all dormancy and ownership properties remain mandatory.
	if unit == "homenode.slice" {
		common["LoadState"] = "loaded"
		if validateRecoveryProperties(data, common) == nil {
			return nil
		}
		common["LoadState"] = "not-found"
	}
	return validateRecoveryProperties(data, common)
}

// configurationRemovalUnits includes every installed unit, not merely runtime
// services. Sockets precede services and the resource slice is stopped last.
func configurationRemovalUnits(plan ConfigurationRemovalPlan) ([]string, error) {
	selected := map[string]bool{}
	for _, file := range plan.Files {
		if strings.HasPrefix(file.Path, "etc/systemd/system/") {
			name := strings.TrimPrefix(file.Path, "etc/systemd/system/")
			if !files[file.Path] || selected[name] {
				return nil, ErrPlan
			}
			selected[name] = true
		}
	}
	order := []string{"homenode-app-maintenance.socket", "homenode-backup-credential.socket", "homenode-gateway.service", "homenode-control.service", "homenode-transfer.service", "homenode-backup.service", "homenode-supervisor.service", "homenode-inspect.service", "homenode.slice"}
	units := []string{}
	for _, name := range order {
		if selected[name] {
			units = append(units, name)
			delete(selected, name)
		}
	}
	if len(selected) != 0 {
		return nil, ErrPlan
	}
	return units, nil
}

// This admission establishes loaded fragment ownership before a stop; it makes
// no claim about dormancy. An absent fragment is never admitted by this parser.
func validateRemovalLoadedUnit(data []byte, unit string) error {
	if !files["etc/systemd/system/"+unit] {
		return ErrPlan
	}
	return validateRecoveryProperties(data, map[string]string{"Job": "", "Id": unit, "DropInPaths": "", "Transient": "no", "LoadState": "loaded", "FragmentPath": "/etc/systemd/system/" + unit, "NeedDaemonReload": "no"})
}

// validateRemovalRetiredUnit is the final manager state after daemon-reload.
// Unlike an intermediate unlink observation, retaining an old loaded fragment
// is not enough: every retired fragment must be absent, inactive and job-free.
// Systemd synthesizes slices without a fragment as loaded units.
func validateRemovalRetiredUnit(data []byte, unit string) error {
	if !files["etc/systemd/system/"+unit] {
		return ErrPlan
	}
	expected := map[string]string{"Job": "", "Id": unit, "DropInPaths": "", "Transient": "no", "LoadState": "not-found", "FragmentPath": "", "NeedDaemonReload": "no", "ActiveState": "inactive", "SubState": "dead"}
	if unit == "homenode.slice" {
		expected["LoadState"] = "loaded"
		if validateRecoveryProperties(data, expected) == nil {
			return nil
		}
		expected["LoadState"] = "not-found"
	}
	if strings.HasSuffix(unit, ".service") {
		expected["MainPID"] = "0"
		expected["ControlPID"] = "0"
	}
	return validateRecoveryProperties(data, expected)
}
