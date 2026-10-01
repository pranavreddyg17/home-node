package updates

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func payloadFixture(t *testing.T, scenario string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	var inventory strings.Builder
	files := []string{"usr/bin/homenode", "usr/lib/homenode/homenode-supervisor", "usr/lib/homenode/homenode-transfer", "usr/lib/homenode/homenode-backup", "usr/lib/homenode/guest/homenode-guest", "usr/share/homenode/web/index.html"}
	if scenario == "duplicate-entry" {
		files = append(files, files[0])
	}
	if scenario == "outside" {
		files = append(files, "etc/passwd")
	}
	if scenario == "missing" {
		files = files[1:]
	}
	for _, name := range files {
		data := []byte("fixture:" + name)
		mode := int64(0644)
		if packageExecutables[name] {
			mode = 0755
		}
		header := &tar.Header{Name: "./" + name, Mode: mode, Size: int64(len(data)), Typeflag: tar.TypeReg}
		if scenario == "writable" {
			header.Mode |= 0022
		}
		if scenario == "foreign-owner" {
			header.Uid = 1000
		}
		if scenario == "link" {
			header.Typeflag = tar.TypeSymlink
			header.Linkname = "/etc/passwd"
			header.Size = 0
		}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if header.Typeflag == tar.TypeReg {
			if _, err := writer.Write(data); err != nil {
				t.Fatal(err)
			}
		}
		fmt.Fprintf(&inventory, "%x  %s\n", sha256.Sum256(data), name)
	}
	manifest := inventory.String()
	if scenario == "corrupt-inventory" {
		manifest = "0" + manifest[1:]
	}
	if scenario == "duplicate-inventory" {
		manifest += manifest
	}
	if err := writer.WriteHeader(&tar.Header{Name: "./" + checksumInventory, Mode: 0644, Size: int64(len(manifest)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte(manifest)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestPayloadInventoryAndInstallationPaths(t *testing.T) {
	for _, scenario := range []string{"valid", "outside", "missing", "writable", "foreign-owner", "link", "corrupt-inventory", "duplicate-inventory", "duplicate-entry"} {
		t.Run(scenario, func(t *testing.T) {
			err := ValidatePayloadArchive(context.Background(), bytes.NewReader(payloadFixture(t, scenario)))
			if (scenario == "valid") != (err == nil) {
				t.Fatal(err)
			}
		})
	}
}

func TestPayloadRejectsTruncatedTrailingAndCanceledInput(t *testing.T) {
	valid := payloadFixture(t, "valid")
	for _, data := range [][]byte{valid[:300], valid[:600], append(append([]byte(nil), valid...), 'x')} {
		if err := ValidatePayloadArchive(context.Background(), bytes.NewReader(data)); err == nil {
			t.Fatal("truncated or trailing payload accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ValidatePayloadArchive(ctx, bytes.NewReader(valid)); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation lost", err)
	}
}
