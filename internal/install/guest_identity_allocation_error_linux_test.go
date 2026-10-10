//go:build linux

package install

import (
	"context"
	"errors"
	"github.com/pranavreddyg17/home-node/internal/accountlock"
	"testing"
)

func TestGuestIdentityAllocationLockErrorPreservesCauseAndClassification(t *testing.T) {
	for _, pair := range []struct{ cause, want error }{{accountlock.ErrInvalid, ErrPlan}, {accountlock.ErrConflict, ErrConflict}, {context.Canceled, context.Canceled}} {
		mapped := mapGuestIdentityAllocationLockError(pair.cause)
		if !errors.Is(mapped, pair.cause) || !errors.Is(mapped, pair.want) {
			t.Fatal("account boundary lost classification or cause", mapped)
		}
	}
	if err := mapGuestIdentityAllocationLockError(nil); err != nil {
		t.Fatal("successful check became refusal", err)
	}
}
