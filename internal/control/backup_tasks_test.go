package control

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBackupTasksShutdownRetainsWorkerOwnership(t *testing.T) {
	tasks := newBackupTasks()
	entered := make(chan struct{})
	release := make(chan struct{})
	if err := tasks.start(func(context.Context) (func(context.Context) error, error) {
		return func(ctx context.Context) error { close(entered); <-ctx.Done(); <-release; return ctx.Err() }, nil
	}); err != nil {
		t.Fatal(err)
	}
	<-entered
	called := false
	if err := tasks.start(func(context.Context) (func(context.Context) error, error) { called = true; return nil, nil }); !errors.Is(err, errBackupTaskUnavailable) || called {
		t.Fatal("overlapping admission executed", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := tasks.close(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("unfinished worker reported complete", err)
	}
	if err := tasks.start(func(context.Context) (func(context.Context) error, error) { called = true; return nil, nil }); !errors.Is(err, errBackupTaskUnavailable) || called {
		t.Fatal("shutdown admitted work", err)
	}
	close(release)
	deadline, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := tasks.close(deadline); err != nil {
		t.Fatal("completed worker not drained", err)
	}
}
func TestBackupTaskAdmissionFailureDoesNotReserveWork(t *testing.T) {
	tasks := newBackupTasks()
	defer tasks.close(context.Background())
	expected := errors.New("fixture admission failure")
	if err := tasks.start(func(context.Context) (func(context.Context) error, error) { return nil, expected }); !errors.Is(err, expected) {
		t.Fatal(err)
	}
	completed := make(chan struct{})
	if err := tasks.start(func(context.Context) (func(context.Context) error, error) {
		return func(context.Context) error { close(completed); return nil }, nil
	}); err != nil {
		t.Fatal("failed admission reserved slot", err)
	}
	<-completed
}
