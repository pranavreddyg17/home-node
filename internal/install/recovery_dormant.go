package install

import (
	"strings"
	"unicode/utf8"
)

// validateRecoveryDormant parses a bounded system manager observation for one
// fixed service. Dormancy is not a retained exclusion lease: production handoff
// must prevent activation and reobserve before publishing any restored file.
func validateRecoveryDormant(data []byte) error {
	expected := map[string]string{"LoadState": "loaded", "ActiveState": "inactive", "SubState": "dead", "MainPID": "0", "ControlPID": "0"}
	return validateRecoveryProperties(data, expected)
}

func validateRecoveryDormantUnit(data []byte, unit string) error {
	switch unit {
	case "homenode-gateway.service", "homenode-control.service", "homenode-transfer.service", "homenode-supervisor.service", "homenode-backup.service", "homenode-backup-credential.socket":
	default:
		return ErrPlan
	}
	expected := map[string]string{"Job": "", "Id": unit, "FragmentPath": "/etc/systemd/system/" + unit, "DropInPaths": "", "NeedDaemonReload": "no", "Transient": "no", "LoadState": "loaded", "ActiveState": "inactive", "SubState": "dead", "MainPID": "0", "ControlPID": "0"}
	if unit == "homenode-backup-credential.socket" {
		delete(expected, "MainPID")
		delete(expected, "ControlPID")
	}
	return validateRecoveryProperties(data, expected)
}

// Loaded identity admission applies before a stop operation, including when
// the owned unit is active. State and process emptiness are checked afterward.
func validateRecoveryLoadedUnit(data []byte, unit string) error {
	switch unit {
	case "homenode-gateway.service", "homenode-control.service", "homenode-transfer.service", "homenode-supervisor.service", "homenode-backup.service", "homenode-backup-credential.socket":
	default:
		return ErrPlan
	}
	return validateRecoveryProperties(data, map[string]string{"Id": unit, "FragmentPath": "/etc/systemd/system/" + unit, "DropInPaths": "", "NeedDaemonReload": "no", "Transient": "no", "Job": "", "LoadState": "loaded"})
}

func validateRecoveryProperties(data []byte, expected map[string]string) error {
	if len(data) == 0 || len(data) > 1024 || !utf8.Valid(data) {
		return ErrConflict
	}
	seen := make(map[string]bool, len(expected))
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		key, value, ok := strings.Cut(line, "=")
		want, known := expected[key]
		if !ok || !known || seen[key] || value != want {
			return ErrConflict
		}
		seen[key] = true
	}
	if len(seen) != len(expected) {
		return ErrConflict
	}
	return nil
}

func recoveryUnits(gateway bool) []string {
	units := []string{"homenode-control.service", "homenode-transfer.service", "homenode-supervisor.service", "homenode-backup.service", "homenode-backup-credential.socket"}
	if gateway {
		units = append([]string{"homenode-gateway.service"}, units...)
	}
	return units
}
func installedGateway(config journal) bool {
	for _, item := range config.Items {
		if item.Path == "etc/systemd/system/homenode-gateway.service" {
			return true
		}
	}
	return false
}
func requestedGateway(values []bool) bool { return len(values) == 1 && values[0] }
