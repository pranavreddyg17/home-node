package install

import (
	"strings"
	"testing"
)

func TestGatewayTLSRetirementStateRefusesSkippedRegressedAndForeignMetadata(t *testing.T) {
	for _, certGID := range []int{803, 0} {
		identity := GatewayAccount{UID: 803, GID: 803, ProxyGID: 804}
		installed := journal{ID: strings.Repeat("a", 32), Digest: strings.Repeat("b", 64), Phase: "installed", Items: []record{{Path: "etc/homenode/tls", Directory: true, UID: 0, GID: 803, Mode: 0750, State: "created"}}}
		key := ConfigurationRemovalFile{Path: "etc/homenode/tls/server.key", GID: 803, Mode: 0640, SHA256: strings.Repeat("c", 64)}
		cert := ConfigurationRemovalFile{Path: "etc/homenode/tls/server.crt", GID: certGID, Mode: 0644, SHA256: strings.Repeat("d", 64)}
		plan, err := gatewayTLSRetirementPlan(installed, identity, key, cert)
		if err != nil {
			t.Fatal(err)
		}
		states := [][]ConfigurationRemovalFile{{key, cert, {Path: "etc/homenode/tls", GID: 803, Mode: 0750}}}
		for _, change := range plan.Changes {
			next := append([]ConfigurationRemovalFile(nil), states[len(states)-1]...)
			for i := range next {
				if next[i].Path == change.Path {
					next[i].Mode, next[i].GID = change.AfterMode, change.AfterGID
				}
			}
			states = append(states, next)
		}
		for completed := 0; completed <= 5; completed++ {
			for phase, state := range states {
				admissible := false
				for candidate := completed; candidate <= completed+1 && candidate <= 5; candidate++ {
					if sameTLSState(states[candidate], state) {
						admissible = true
					}
				}
				_, err := gatewayTLSRetirementState(plan, completed, false, state)
				if (err == nil) != admissible {
					t.Fatalf("gid=%d acknowledged=%d observed=%d admitted=%v want=%v", certGID, completed, phase, err == nil, admissible)
				}
			}
		}
		if _, err := gatewayTLSRetirementState(plan, 5, true, states[5]); err != nil {
			t.Fatal(err)
		}
		for _, index := range []int{0, 1, 2} {
			bad := append([]ConfigurationRemovalFile(nil), states[5]...)
			bad[index].UID = 803
			if _, err := gatewayTLSRetirementState(plan, 5, true, bad); err == nil {
				t.Fatal("foreign owner admitted")
			}
		}
		bad := append([]ConfigurationRemovalFile(nil), states[0]...)
		bad[0].SHA256 = strings.Repeat("e", 64)
		if _, err := gatewayTLSRetirementState(plan, 0, false, bad); err == nil {
			t.Fatal("changed key contents admitted")
		}
		plan.Changes[0].AfterMode = 0644
		if _, err := gatewayTLSRetirementState(plan, 0, false, states[0]); err == nil {
			t.Fatal("foreign permission operation admitted")
		}
	}
}
func sameTLSState(a, b []ConfigurationRemovalFile) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
