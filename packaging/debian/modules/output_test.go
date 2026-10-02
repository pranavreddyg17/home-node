package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

type failingEvidenceWriter struct{ failure error }

func (w failingEvidenceWriter) Write(data []byte) (int, error) { return len(data) / 2, w.failure }

func TestEvidenceOutputFailureAndBounds(t *testing.T) {
	failure := errors.New("output unavailable")
	if err := writeEvidence(failingEvidenceWriter{failure}, map[string]int{"schema": 1}); !errors.Is(err, failure) {
		t.Fatal("output failure lost", err)
	}
	if err := writeEvidence(failingEvidenceWriter{}, map[string]int{"schema": 1}); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal("truncated evidence reported success", err)
	}
	var output bytes.Buffer
	if err := writeEvidence(&output, strings.Repeat("x", 8<<20)); err == nil || output.Len() != 0 {
		t.Fatal("oversized evidence published", err)
	}
	if err := writeEvidence(&output, map[string]int{"schema": 1}); err != nil || output.String() != "{\"schema\":1}\n" {
		t.Fatal("canonical complete evidence output failed", err)
	}
}
