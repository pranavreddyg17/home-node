package updates

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestCompletePackageInspectionRequiresBothControlAndPayload(t *testing.T) {
	control := "Package: homenode\nVersion: 0.1.0\nArchitecture: amd64\nMaintainer: fixture\nDepends: " + homeNodeDependencies + "\nDescription: fixture\n"
	release := ReleaseMetadata{Release: "0.1.0", Platform: "ubuntu-24.04-amd64"}
	for _, compression := range []string{"none", "gzip", "zstd"} {
		for _, scenario := range []string{"valid", "corrupt-inventory", "install-hook", "wrong-version"} {
			t.Run(compression+"/"+scenario, func(t *testing.T) {
				payloadScenario, hook := "valid", ""
				if scenario == "corrupt-inventory" {
					payloadScenario = scenario
				}
				if scenario == "install-hook" {
					hook = "./postinst"
				}
				suffix := map[string]string{"none": "", "gzip": ".gz", "zstd": ".zst"}[compression]
				members := []struct {
					name string
					data []byte
				}{
					{"debian-binary", []byte("2.0\n")},
					{"control.tar" + suffix, compressControlFixture(t, controlFixture(t, control, hook), compression)},
					{"data.tar" + suffix, compressControlFixture(t, payloadFixture(t, payloadScenario), compression)},
				}
				var archive bytes.Buffer
				archive.WriteString("!<arch>\n")
				for _, member := range members {
					fmt.Fprintf(&archive, "%-16s%-12s%-6s%-6s%-8s%-10d`\n", member.name, "0", "0", "0", "100644", len(member.data))
					archive.Write(member.data)
					if len(member.data)%2 != 0 {
						archive.WriteByte('\n')
					}
				}
				filename := filepath.Join(t.TempDir(), "release.deb")
				if err := os.WriteFile(filename, archive.Bytes(), 0400); err != nil {
					t.Fatal(err)
				}
				file, err := os.Open(filename)
				if err != nil {
					t.Fatal(err)
				}
				defer file.Close()
				metadata := release
				if scenario == "wrong-version" {
					metadata.Release = "0.2.0"
				}
				if err = ValidateDebianContent(context.Background(), file, metadata); (scenario == "valid") != (err == nil) {
					t.Fatal("incorrect whole-package result", err)
				}
			})
		}
	}
}
