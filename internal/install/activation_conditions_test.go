package install

import (
	"errors"
	"strings"
	"testing"
)

func TestActivationConditionsRequireLoadedOrdinaryNegatedGuard(t *testing.T) {
	valid := `{"type":"a(sbbsi)","data":[["ConditionPathExists",false,true,"/var/lib/homenode-install/recovery-blocked",0]]}`
	for _, result := range []string{"0", "1", "-1"} {
		if err := validateActivationConditions([]byte(strings.Replace(valid, ",0]", ","+result+"]", 1))); err != nil {
			t.Fatal("previous condition result treated as authority", err)
		}
	}
	for _, invalid := range []string{
		strings.Replace(valid, "false,true", "true,true", 1),
		strings.Replace(valid, "false,true", "false,false", 1),
		strings.Replace(valid, "recovery-blocked", "foreign", 1),
		strings.Replace(valid, "ConditionPathExists", "ConditionPathIsDirectory", 1),
		strings.Replace(valid, "false,true", "null,true", 1),
		strings.Replace(valid, ",0]", ",null]", 1),
		strings.Replace(valid, ",0]", ",2147483648]", 1),
		strings.Replace(valid, `"type":`, `"type":"a(sbbsi)","type":`, 1),
		strings.Replace(valid, `"data":`, `"foreign":0,"data":`, 1),
		valid + valid,
		`{"type":"a(sbbsi)","data":[]}`,
	} {
		if err := validateActivationConditions([]byte(invalid)); !errors.Is(err, ErrConflict) {
			t.Fatal("unsafe loaded condition admitted", invalid, err)
		}
	}
}
