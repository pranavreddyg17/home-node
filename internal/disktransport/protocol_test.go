package disktransport

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDiskRequestRejectsAmbiguousAuthority(t *testing.T) {
	id := strings.Repeat("a", 24)
	valid := `{"version":1,"token":"` + id + `","instanceId":"` + id + `"}`
	if _, err := DecodeRequest([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{valid + `{}`, `null`, strings.Replace(valid, `"version":1`, `"version":1,"version":1`, 1), strings.Replace(valid, `"token":"`+id+`"`, `"token":"`+id+`","to\u006ben":"`+id+`"`, 1), strings.Replace(valid, `"version":1`, `"version":null`, 1), strings.Replace(valid, `"version":1`, `"version":2`, 1), strings.Replace(valid, `"token":"`+id+`"`, `"token":"short"`, 1), strings.Replace(valid, `"version":1`, `"extra":1,"version":1`, 1), `{"version":1}`} {
		if _, err := DecodeRequest([]byte(data)); err == nil {
			t.Fatal("invalid authority accepted", data)
		}
	}
}
func TestDiskMetadataRequiresSupportedInventory(t *testing.T) {
	m := Metadata{Version: 1, InstanceID: strings.Repeat("a", 24), Workload: "files", Bytes: 16 << 20, ImageSHA256: strings.Repeat("a", 64), DataSchema: 1, Protocol: 1}
	data, _ := json.Marshal(m)
	if decoded, err := DecodeMetadata(data); err != nil || decoded != m {
		t.Fatal(decoded, err)
	}
	for _, kind := range []string{"version", "id", "workload", "bytes-small", "bytes-large", "digest", "schema", "protocol"} {
		changed := m
		switch kind {
		case "version":
			changed.Version = 2
		case "id":
			changed.InstanceID = "short"
		case "workload":
			changed.Workload = "video"
		case "bytes-small":
			changed.Bytes = 1
		case "bytes-large":
			changed.Bytes = 513 << 30
		case "digest":
			changed.ImageSHA256 = "bad"
		case "schema":
			changed.DataSchema = 2
		case "protocol":
			changed.Protocol = 2
		}
		data, _ := json.Marshal(changed)
		if _, err := DecodeMetadata(data); err == nil {
			t.Fatal("incompatible inventory admitted", kind)
		}
	}
	data, _ = json.Marshal(m)
	for _, bad := range [][]byte{append(data, []byte("null")...), []byte(strings.Replace(string(data), `"bytes":16777216`, `"bytes":null`, 1)), []byte(strings.Replace(string(data), `"bytes":16777216`, `"bytes":16777216,"b\u0079tes":16777216`, 1)), append(data, 0xff)} {
		if _, err := DecodeMetadata(bad); err == nil {
			t.Fatal("ambiguous metadata accepted")
		}
	}
}
