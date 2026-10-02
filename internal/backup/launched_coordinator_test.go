package backup

import (
	"context"
	"os"
	"testing"
)

func TestLaunchedCoordinatorRejectsIncompleteDependencies(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "credential")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	called := false
	launch := func(context.Context, Launch, *os.File) error { called = true; return nil }
	cleanup := func(context.Context, Cleanup, *os.File) error { called = true; return nil }
	if _, err = RunAdmittedLaunchedMaintenance(context.Background(), nil, "", "", "", "0.1.0", 1, file, &coordinatorApps{}, launch, cleanup); err == nil || called {
		t.Fatal("incomplete coordinator performed work", err)
	}
	if _, err = file.Stat(); err != nil {
		t.Fatal("caller credential closed", err)
	}
}
