//go:build !linux

package install

import "testing"

func applyAccountFixtureIdentity(t *testing.T, _ **Engine, _ GuestIdentityConfigurationPreview) {
	t.Helper()
	t.Fatal("native identity fixture requires Linux")
}
