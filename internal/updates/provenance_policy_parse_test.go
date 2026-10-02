package updates

import (
	"strings"
	"testing"
)

func TestProvenancePolicyRequiresDistinctThresholdKeysAndExactConfiguration(t *testing.T) {
	first, second := strings.Repeat("ab", 32), strings.Repeat("cd", 32)
	data := `{"schema":1,"threshold":2,"keys":["` + first + `","` + second + `"],"builderId":"builder","buildType":"type","externalParameters":{},"sourceUri":"source","sourceCommit":"` + strings.Repeat("ef", 20) + `"}`
	policy, err := ParseProvenancePolicy([]byte(data))
	if err != nil || policy.Threshold != 2 || len(policy.Keys) != 2 {
		t.Fatal("reviewed policy refused", err)
	}
	for _, invalid := range []string{
		strings.Replace(data, second, first, 1),
		strings.Replace(data, `"threshold":2`, `"threshold":1`, 1),
		strings.Replace(data, `"threshold":2`, `"threshold":3`, 1),
		strings.Replace(data, `"builderId"`, `"BuilderId"`, 1),
		strings.Replace(data, `"externalParameters":{}`, `"externalParameters":null`, 1),
		strings.Replace(data, first, strings.ToUpper(first), 1),
		strings.Replace(data, `"schema":1`, `"schema":1,"unexpected":true`, 1),
		strings.Repeat(" ", 16385) + data,
	} {
		refused, err := ParseProvenancePolicy([]byte(invalid))
		if err == nil || len(refused.Keys) != 0 || refused.Threshold != 0 {
			t.Fatal("invalid trust policy exposed keys", err)
		}
	}
}
