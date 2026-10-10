//go:build linux

package install

import (
	"context"
	"errors"
	"testing"
)

func TestGuestStorageBootIdentityRequiresCanonicalKernelUUID(t *testing.T) {
	valid := "12345678-1234-1234-1234-123456789abc"
	if !guestStorageBootIDAdmitted(valid) {
		t.Fatal("canonical boot identity refused")
	}
	for _, value := range []string{"", valid + "\n", "00000000-0000-0000-0000-000000000000", "12345678-1234-1234-1234-123456789abC", "1234567811234-1234-1234-123456789abc", "12345678-1234-1234-1234-123456789abg"} {
		if guestStorageBootIDAdmitted(value) {
			t.Fatal("foreign boot representation admitted", value)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if value, err := observeGuestStorageBootID(ctx); !errors.Is(err, context.Canceled) || value != "" {
		t.Fatal("cancelled observation returned boot authority", value, err)
	}
}
