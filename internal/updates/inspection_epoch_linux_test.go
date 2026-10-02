//go:build linux

package updates

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestInspectionLaunchEpochBindsLiveBoot(t *testing.T) {
	epoch, err := CaptureInspectionLaunchEpoch(context.Background())
	if err != nil || !validInspectionBootID(epoch.BootID) || epoch.NotBeforeMicros == 0 {
		t.Fatal("live epoch unavailable", epoch, err)
	}
	if err := VerifyInspectionLaunchEpoch(context.Background(), epoch); err != nil {
		t.Fatal(err)
	}
	wrong := epoch
	wrong.BootID = "01234567-89ab-cdef-0123-456789abcdef"
	if wrong.BootID == epoch.BootID {
		wrong.BootID = "fedcba98-7654-3210-fedc-ba9876543210"
	}
	if err := VerifyInspectionLaunchEpoch(context.Background(), wrong); err == nil {
		t.Fatal("other boot admitted")
	}
	future := epoch
	future.NotBeforeMicros = ^uint64(0)
	if err := VerifyInspectionLaunchEpoch(context.Background(), future); err == nil {
		t.Fatal("future boundary admitted")
	}
	for _, boot := range []string{"", strings.Repeat("a", 4096), "01234567-89AB-CDEF-0123-456789ABCDEF", "00000000-0000-0000-0000-000000000000", epoch.BootID + "\n"} {
		if validInspectionBootID(boot) {
			t.Fatal("ambiguous boot identity admitted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if captured, err := CaptureInspectionLaunchEpoch(ctx); captured != (InspectionLaunchEpoch{}) || !errors.Is(err, context.Canceled) {
		t.Fatal("canceled capture admitted", captured, err)
	}
	if err := VerifyInspectionLaunchEpoch(ctx, epoch); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled verification admitted", err)
	}
}
