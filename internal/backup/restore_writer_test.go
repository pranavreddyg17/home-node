//go:build linux || darwin

package backup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"testing"
)

func TestRestoreWriterRefusesOversizedOutput(t *testing.T) {
	stage, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer stage.Close()
	var destination bytes.Buffer
	writer := &restoreWriter{ctx: context.Background(), stage: stage, destination: &destination, remaining: 3}
	if _, err := writer.Write([]byte("too many bytes")); err == nil || destination.Len() != 0 {
		t.Fatal("oversized restore wrote data")
	}
	if _, err := writer.Write([]byte("abc")); err != nil || writer.remaining != 0 {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("d")); err == nil {
		t.Fatal("restore grew past manifest size")
	}
}

func TestRestoreWriterCapacityRefusalDoesNotWriteOrConsumeDeclaredBytes(t *testing.T) {
	var destination bytes.Buffer
	writer := &restoreWriter{ctx: context.Background(), destination: &destination, remaining: 3}
	if n, err := writer.Write([]byte("abc")); n != 0 || !errors.Is(err, ErrStagingCapacity) {
		t.Fatal("missing capacity observation accepted", n, err)
	}
	if destination.Len() != 0 || writer.remaining != 3 {
		t.Fatal("capacity refusal wrote or consumed bytes")
	}
	stage, err := os.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := stage.Close(); err != nil {
		t.Fatal(err)
	}
	writer.stage = stage
	if n, err := writer.Write([]byte("abc")); n != 0 || !errors.Is(err, ErrStagingCapacity) {
		t.Fatal("closed capacity descriptor accepted", n, err)
	}
	if destination.Len() != 0 || writer.remaining != 3 {
		t.Fatal("closed-descriptor refusal wrote or consumed bytes")
	}
}

func TestRestoreWriterCancellationPreventsOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var destination bytes.Buffer
	writer := &restoreWriter{ctx: ctx, destination: &destination, remaining: 3}
	if n, err := writer.Write([]byte("abc")); n != 0 || !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled writer accepted output", n, err)
	}
	if destination.Len() != 0 || writer.remaining != 3 {
		t.Fatal("cancelled writer consumed declared bytes")
	}
}
