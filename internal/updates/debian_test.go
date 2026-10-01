package updates

import (
	"bytes"
	"fmt"
	"io"
	"testing"
)

func debianFixture(controlName, dataName string) []byte {
	var buffer bytes.Buffer
	buffer.WriteString("!<arch>\n")
	for _, item := range [][2]string{{"debian-binary", "2.0\n"}, {controlName, "control"}, {dataName, "payload"}} {
		fmt.Fprintf(&buffer, "%-16s%-12s%-6s%-6s%-8s%-10d`\n", item[0], "0", "0", "0", "100644", len(item[1]))
		buffer.WriteString(item[1])
		if len(item[1])%2 != 0 {
			buffer.WriteByte('\n')
		}
	}
	return buffer.Bytes()
}

func TestDebianArchiveBoundsMembersWithoutExtracting(t *testing.T) {
	for _, suffix := range []string{"", ".gz", ".xz", ".zst"} {
		data := debianFixture("control.tar"+suffix, "data.tar"+suffix)
		archive, err := inspectDebianArchive(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(suffix, err)
		}
		control, err := io.ReadAll(archive.Control)
		if err != nil || string(control) != "control" {
			t.Fatal(string(control), err)
		}
		payload, err := io.ReadAll(archive.Data)
		if err != nil || string(payload) != "payload" {
			t.Fatal(string(payload), err)
		}
	}
	valid := debianFixture("control.tar.xz", "data.tar.xz")
	cases := map[string][]byte{
		"bad-magic":           append([]byte("invalid!"), valid[8:]...),
		"wrong-order":         debianFixture("data.tar.xz", "control.tar.xz"),
		"unknown-compression": debianFixture("control.tar.bz2", "data.tar.xz"),
		"trailing-member":     append(append([]byte(nil), valid...), []byte("unexpected")...),
		"truncated-header":    valid[:30],
		"truncated-payload":   valid[:len(valid)-2],
		"format-version":      bytes.Replace(valid, []byte("2.0\n"), []byte("3.0\n"), 1),
	}
	for name, position := range map[string]int{"bad-size": 8 + 48, "bad-header-end": 8 + 58, "bad-padding": 8 + 60 + 4 + 60 + 7} {
		data := append([]byte(nil), valid...)
		data[position] = '!'
		cases[name] = data
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := inspectDebianArchive(bytes.NewReader(data), int64(len(data))); err == nil {
				t.Fatal("invalid archive accepted")
			}
		})
	}
	if _, err := InspectDebianArchive(nil); err == nil {
		t.Fatal("missing descriptor accepted")
	}
	if _, err := inspectDebianArchive(bytes.NewReader(valid), 513<<20); err == nil {
		t.Fatal("oversized archive accepted")
	}
}
