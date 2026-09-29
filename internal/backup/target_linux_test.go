//go:build linux

package backup

import "testing"

func TestMissingRegisteredDriveNeverOpensHostDirectory(t *testing.T) {
	target := registeredTarget()
	target.MountPath = t.TempDir()
	target.UUID = "homenode-nonexistent-test-drive"
	if directory, err := OpenTarget(target); err == nil {
		directory.Close()
		t.Fatal("missing drive fell back to host directory")
	}
}
