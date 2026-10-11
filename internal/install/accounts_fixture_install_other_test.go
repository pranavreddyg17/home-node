//go:build !linux

package install

import "testing"

func installAccountFixtureConfiguration(t *testing.T, _ *Engine, _ Accounts, _ MaintenanceAccount, _ ...*GatewayAccount) {
	t.Helper()
	t.Fatal("owned installation fixture requires Linux")
}
