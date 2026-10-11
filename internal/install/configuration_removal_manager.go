package install

// validateRemovalDormantUnit admits only the owned uninstall transition. Missing
// is supplied from canonical intent and filesystem observation, never from the
// manager response alone. Ordinary recovery continues requiring loaded units
// with no pending reload.
func validateRemovalDormantUnit(data []byte, unit string, missing bool) error {
	switch unit {
	case "homenode-gateway.service", "homenode-control.service", "homenode-transfer.service", "homenode-supervisor.service", "homenode-backup.service", "homenode-backup-credential.socket":
	default:
		return ErrPlan
	}
	if !missing {
		return validateRecoveryDormantUnit(data, unit)
	}
	common := map[string]string{"Job": "", "Id": unit, "DropInPaths": "", "Transient": "no", "ActiveState": "inactive", "SubState": "dead", "MainPID": "0", "ControlPID": "0"}
	if unit == "homenode-backup-credential.socket" {
		delete(common, "MainPID")
		delete(common, "ControlPID")
	}
	// The running manager may retain the previously loaded, now absent fragment
	// until daemon-reload. Its path must remain exactly the owned unit path.
	common["LoadState"] = "loaded"
	common["FragmentPath"] = "/etc/systemd/system/" + unit
	for _, reload := range []string{"yes", "no"} {
		common["NeedDaemonReload"] = reload
		if validateRecoveryProperties(data, common) == nil {
			return nil
		}
	}
	// After reload (or manager collection), only a genuinely absent, inactive
	// unit is accepted. A masked/failed/generated/aliased unit is not absence.
	common["LoadState"] = "not-found"
	common["FragmentPath"] = ""
	common["NeedDaemonReload"] = "no"
	return validateRecoveryProperties(data, common)
}
