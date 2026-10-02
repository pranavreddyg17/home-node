package updates

import (
	"strings"
	"testing"
)

func TestInspectionProcessPolicyRejectsBroaderNamespacesSocketsAndABI(t *testing.T) {
	properties := "RestrictNamespaces=yes\nRestrictAddressFamilies=AF_UNIX\nSystemCallArchitectures=native\nStandardOutput=journal\nStandardError=journal\n"
	if err := ValidateInspectionProcessPolicy([]byte(properties)); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"RestrictNamespaces=yes", "RestrictNamespaces=no"}, {"RestrictAddressFamilies=AF_UNIX", "RestrictAddressFamilies=AF_UNIX AF_INET"}, {"SystemCallArchitectures=native", "SystemCallArchitectures=native x86"}, {"StandardOutput=journal", "StandardOutput=inherit"}, {"StandardError=journal", "StandardError=null"}} {
		if err := ValidateInspectionProcessPolicy([]byte(strings.Replace(properties, pair[0], pair[1], 1))); err == nil {
			t.Fatal("broader process policy accepted", pair)
		}
	}
	for _, data := range []string{properties + "RestrictNamespaces=yes\n", properties + "Unknown=yes\n", strings.Replace(properties, "RestrictNamespaces=yes\n", "", 1), strings.Replace(properties, "StandardOutput=journal\n", "", 1), strings.Replace(properties, "StandardError=journal\n", "", 1), properties + "StandardOutput=journal\n", strings.Repeat("x", 2049)} {
		if err := ValidateInspectionProcessPolicy([]byte(data)); err == nil {
			t.Fatal("ambiguous process policy accepted")
		}
	}
}
