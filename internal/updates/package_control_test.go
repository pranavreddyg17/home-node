package updates

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDebianControlInspectionUsesDescriptorAndPreservesOffset(t *testing.T) {
	control := "Package: homenode\nVersion: 0.1.0\nArchitecture: amd64\nMaintainer: fixture\nDepends: " + homeNodeDependencies + "\nDescription: fixture\n"
	release := ReleaseMetadata{Release: "0.1.0", Platform: "ubuntu-24.04-amd64"}
	for _, scenario := range []string{"valid", "wrong-version", "install-hook", "truncated"} {
		t.Run(scenario, func(t *testing.T) {
			extra := ""
			if scenario == "install-hook" {
				extra = "./postinst"
			}
			compressed := compressControlFixture(t, controlFixture(t, control, extra), "gzip")
			var archive bytes.Buffer
			archive.WriteString("!<arch>\n")
			for _, item := range []struct {
				name string
				data []byte
			}{{"debian-binary", []byte("2.0\n")}, {"control.tar.gz", compressed}, {"data.tar.gz", []byte("unqualified payload fixture")}} {
				fmt.Fprintf(&archive, "%-16s%-12s%-6s%-6s%-8s%-10d`\n", item.name, "0", "0", "0", "100644", len(item.data))
				archive.Write(item.data)
				if len(item.data)%2 != 0 {
					archive.WriteByte('\n')
				}
			}
			data := archive.Bytes()
			if scenario == "truncated" {
				data = data[:len(data)-2]
			}
			filename := filepath.Join(t.TempDir(), "release.deb")
			if err := os.WriteFile(filename, data, 0400); err != nil {
				t.Fatal(err)
			}
			file, err := os.Open(filename)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			if _, err = file.Seek(7, 0); err != nil {
				t.Fatal(err)
			}
			// Inspection must remain on the opened inode, even if the ambient
			// pathname is replaced before inspection starts.
			if err = os.Remove(filename); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filename, []byte("replacement"), 0400); err != nil {
				t.Fatal(err)
			}
			metadata := release
			if scenario == "wrong-version" {
				metadata.Release = "0.2.0"
			}
			err = ValidateDebianControl(context.Background(), file, metadata)
			if (scenario == "valid") != (err == nil) {
				t.Fatal("unexpected control result", err)
			}
			position, seekErr := file.Seek(0, 1)
			if seekErr != nil || position != 7 {
				t.Fatal("inspection changed descriptor position", position, seekErr)
			}
		})
	}
}
