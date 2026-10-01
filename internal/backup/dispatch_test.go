package backup

import (
	"errors"
	"strings"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/state"
)

func TestDispatchStrictOwnershipContract(t *testing.T) {
	d := Dispatch{Version: 1, JobID: state.Random(), DeviceID: state.Random(), ManagementToken: state.Random(), RuntimeToken: state.Random(), Release: "0.1.0", CatalogVersion: 1}
	raw, err := EncodeDispatch(d)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeDispatch(raw)
	if err != nil || decoded != d {
		t.Fatal("dispatch roundtrip", err)
	}
	valid := string(raw)
	for _, invalid := range []string{
		valid + `{}`, valid + strings.Repeat(" ", MaxDispatchBytes), valid + string([]byte{255}),
		strings.Replace(valid, `"version":1`, `"version":1,"vers\u0069on":1`, 1),
		strings.Replace(valid, `"version":1`, `"version":null`, 1),
		strings.Replace(valid, `"version":1`, `"version":2`, 1),
		strings.Replace(valid, `"version":1,`, "", 1),
		strings.Replace(valid, `"version":1`, `"version":1,"path":"/etc/shadow"`, 1),
		strings.Replace(valid, d.JobID, "short", 1),
		strings.Replace(valid, `"catalogVersion":1`, `"catalogVersion":0`, 1),
		strings.Replace(valid, `"catalogVersion":1`, `"catalogVersion":"1"`, 1),
		strings.Replace(valid, `"release":"0.1.0"`, `"release":"../executable"`, 1),
	} {
		decoded, err := DecodeDispatch([]byte(invalid))
		if !errors.Is(err, ErrManifest) || decoded != (Dispatch{}) {
			t.Fatal("untrusted dispatch accepted")
		}
	}
}
