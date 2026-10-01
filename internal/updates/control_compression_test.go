package updates

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func compressControlFixture(t *testing.T, data []byte, compression string) []byte {
	t.Helper()
	if compression == "none" {
		return data
	}
	var buffer bytes.Buffer
	if compression == "gzip" {
		writer := gzip.NewWriter(&buffer)
		if _, err := writer.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		writer, err := zstd.NewWriter(&buffer, zstd.WithEncoderConcurrency(1))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = writer.Write(data); err != nil {
			t.Fatal(err)
		}
		if err = writer.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return buffer.Bytes()
}

func TestCompressedControlRequiresCompleteBoundedStream(t *testing.T) {
	control := "Package: homenode\nVersion: 0.1.0\nArchitecture: amd64\nMaintainer: fixture\nDepends: " + homeNodeDependencies + "\nDescription: fixture\n"
	data := controlFixture(t, control, "")
	release := ReleaseMetadata{Release: "0.1.0", Platform: "ubuntu-24.04-amd64"}
	for _, compression := range []string{"none", "gzip", "zstd"} {
		t.Run(compression, func(t *testing.T) {
			compressed := compressControlFixture(t, data, compression)
			if err := ValidateCompressedControl(context.Background(), bytes.NewReader(compressed), compression, release); err != nil {
				t.Fatal(err)
			}
			bomb := compressControlFixture(t, append(append([]byte(nil), data...), make([]byte, 1<<20)...), compression)
			if err := ValidateCompressedControl(context.Background(), bytes.NewReader(bomb), compression, release); err == nil {
				t.Fatal("unbounded decoded control accepted")
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if err := ValidateCompressedControl(ctx, bytes.NewReader(compressed), compression, release); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if compression != "none" {
				if err := ValidateCompressedControl(context.Background(), bytes.NewReader(compressed[:len(compressed)-1]), compression, release); err == nil {
					t.Fatal("truncated compressed stream accepted")
				}
			}
		})
	}
	if err := ValidateCompressedControl(context.Background(), bytes.NewReader(data), "xz", release); err == nil {
		t.Fatal("unconstrained xz accepted")
	}
}
