package updates

import (
	"bytes"
	"context"
	"testing"
)

func TestCompressedPayloadInventoryAndCompleteStream(t *testing.T) {
	for _, compression := range []string{"none", "gzip", "zstd"} {
		t.Run(compression, func(t *testing.T) {
			valid := compressControlFixture(t, payloadFixture(t, "valid"), compression)
			if err := ValidateCompressedPayload(context.Background(), bytes.NewReader(valid), compression); err != nil {
				t.Fatal(err)
			}
			bad := compressControlFixture(t, payloadFixture(t, "corrupt-inventory"), compression)
			if err := ValidateCompressedPayload(context.Background(), bytes.NewReader(bad), compression); err == nil {
				t.Fatal("corrupt compressed inventory accepted")
			}
			if compression != "none" {
				if err := ValidateCompressedPayload(context.Background(), bytes.NewReader(valid[:len(valid)-1]), compression); err == nil {
					t.Fatal("truncated stream accepted")
				}
			}
		})
	}
}
