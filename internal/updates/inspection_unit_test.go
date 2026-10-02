package updates

import (
	"strings"
	"testing"
)

func TestInspectionUnitIdentityRefusesSubstitutedOrStaleManagerConfiguration(t *testing.T) {
	snapshot := "Id=homenode-inspect.service\nLoadState=loaded\nFragmentPath=/etc/systemd/system/homenode-inspect.service\nDropInPaths=\nNeedDaemonReload=no\nType=oneshot\nRemainAfterExit=yes\nDynamicUser=yes\nTransient=no\n"
	if err := ValidateInspectionUnitIdentity([]byte(snapshot)); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"Id=homenode-inspect.service", "Id=other.service"}, {"LoadState=loaded", "LoadState=masked"}, {"FragmentPath=/etc/systemd/system/homenode-inspect.service", "FragmentPath=/run/systemd/transient/homenode-inspect.service"}, {"DropInPaths=", "DropInPaths=/etc/systemd/system/service.d/override.conf"}, {"NeedDaemonReload=no", "NeedDaemonReload=yes"}, {"Type=oneshot", "Type=simple"}, {"RemainAfterExit=yes", "RemainAfterExit=no"}, {"DynamicUser=yes", "DynamicUser=no"}, {"Transient=no", "Transient=yes"}} {
		if err := ValidateInspectionUnitIdentity([]byte(strings.Replace(snapshot, pair[0], pair[1], 1))); err == nil {
			t.Fatal("unsafe manager load accepted", pair)
		}
	}
	for _, data := range []string{snapshot + "Type=oneshot\n", snapshot + "Unknown=1\n", strings.Replace(snapshot, "DynamicUser=yes\n", "", 1), strings.Replace(snapshot, "loaded", "loaded\r", 1), strings.Repeat("x", 2049)} {
		if err := ValidateInspectionUnitIdentity([]byte(data)); err == nil {
			t.Fatal("ambiguous load snapshot accepted")
		}
	}
}
