package updates

import (
	"strings"
	"testing"
)

func TestInspectionAccessPolicyRefusesIPExceptionsAndExposedHostPaths(t *testing.T) {
	properties := "IPAddressDeny=0.0.0.0/0 ::/0\nIPAddressAllow=\nInaccessiblePaths=-/etc/homenode -/var/lib/homenode -/var/lib/homenode-update -/var/lib/homenode-backup -/run/homenode -/run/homenode-transfer\n"
	if err := ValidateInspectionAccessPolicy([]byte(properties)); err != nil {
		t.Fatal(err)
	}
	reordered := strings.Replace(properties, "0.0.0.0/0 ::/0", "::/0 0.0.0.0/0", 1)
	if err := ValidateInspectionAccessPolicy([]byte(reordered)); err != nil {
		t.Fatal("equivalent deny order refused", err)
	}
	for _, pair := range [][2]string{{"IPAddressAllow=", "IPAddressAllow=127.0.0.1/32"}, {"0.0.0.0/0 ::/0", "0.0.0.0/0"}, {"0.0.0.0/0", "0.0.0.0/1"}, {"0.0.0.0/0", "1.2.3.4/0"}, {"-/etc/homenode ", ""}, {"-/run/homenode-transfer", "-/run/homenode-transfer -/run/homenode-transfer"}, {"-/etc/homenode", "-/etc/../etc/homenode"}} {
		if err := ValidateInspectionAccessPolicy([]byte(strings.Replace(properties, pair[0], pair[1], 1))); err == nil {
			t.Fatal("unsafe access policy accepted", pair)
		}
	}
	if err := ValidateInspectionAccessPolicy([]byte(properties + "IPAddressAllow=\n")); err == nil {
		t.Fatal("duplicate policy accepted")
	}
}
