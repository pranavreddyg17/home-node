package main

import "testing"

func TestBackupExecutionConfigurationRefusesInvalidInstalledInputs(t *testing.T) {
	for _, scenario := range []string{"unavailable", "repository", "release", "catalog", "gid-low", "gid-high", "relative", "unclean", "alias"} {
		repository, release, socket := "registered", "0.1.0", "/run/homenode-backup/credential.sock"
		catalog := int64(1)
		gid := 351
		available := true
		other := "/run/homenode-backup/apps.sock"
		switch scenario {
		case "unavailable":
			available = false
		case "repository":
			repository = ""
		case "release":
			release = "invalid"
		case "catalog":
			catalog = 0
		case "gid-low":
			gid = 0
		case "gid-high":
			gid = 1000
		case "relative":
			socket = "relative"
		case "unclean":
			socket = "/run/../run/credential.sock"
		case "alias":
			other = socket
		}
		if config, err := backupExecutionConfiguration(repository, release, catalog, socket, gid, available, other); err == nil || config != nil {
			t.Fatal("unsafe configuration admitted", scenario, config, err)
		}
	}
	config, err := backupExecutionConfiguration("registered", "0.1.0", 1, "/run/homenode-backup/credential.sock", 351, true, "/run/homenode-backup/apps.sock")
	if err != nil || config == nil || config.Launch == nil || config.Cleanup == nil || config.Release != "0.1.0" || config.CatalogVersion != 1 {
		t.Fatal(config, err)
	}
	config, err = backupExecutionConfiguration("", "", 0, "", 0, false)
	if err != nil || config != nil {
		t.Fatal("disabled execution requires privileged provisioning", config, err)
	}
}
