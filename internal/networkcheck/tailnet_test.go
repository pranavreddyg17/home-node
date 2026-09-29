package networkcheck

import (
	"bytes"
	"testing"
)

func TestTailnetIdentityMustMatchRunningSelf(t *testing.T) {
	c := Config{Bind: "100.100.1.2", Port: 8787, Origin: "https://home.example.ts.net:8787"}
	valid := []byte(`{"BackendState":"Running","TailscaleIPs":["100.100.1.2"],"Self":{"DNSName":"home.example.ts.net."},"futureField":true}`)
	if err := validateTailnet(c, valid); err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{
		bytes.Replace(valid, []byte("Running"), []byte("NeedsLogin"), 1),
		bytes.Replace(valid, []byte("home.example.ts.net."), []byte("another.example.ts.net."), 1),
		bytes.Replace(valid, []byte("100.100.1.2"), []byte("100.100.1.3"), 1),
		[]byte(`{"BackendState":"Running","TailscaleIPs":["100.100.1.2"],"Self":null}`),
		append(append([]byte{}, valid...), []byte(`{}`)...),
		bytes.Repeat([]byte("x"), maxStatusBytes+1),
	} {
		if err := validateTailnet(c, data); err == nil {
			t.Fatal("invalid daemon identity accepted")
		}
	}
	var output statusOutput
	if _, err := output.Write(bytes.Repeat([]byte("x"), maxStatusBytes+1)); err == nil || output.Len() != 0 {
		t.Fatal("unbounded CLI output")
	}
}
