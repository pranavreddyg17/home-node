package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestReviewedSumFileBoundsAndIdentity(t *testing.T) {
	directory := t.TempDir()
	name := filepath.Join(directory, "go.sum")
	expected := []byte("fixture.example/dep v1.0.0 h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n")
	if err := os.WriteFile(name, expected, 0600); err != nil {
		t.Fatal(err)
	}
	data, err := readReviewedSums(name)
	if err != nil || !bytes.Equal(data, expected) {
		t.Fatal("reviewed sum bytes changed", err)
	}
	link := filepath.Join(directory, "link")
	if err := os.Symlink(name, link); err != nil {
		t.Fatal(err)
	}
	if data, err := readReviewedSums(link); err == nil || data != nil {
		t.Fatal("sum symlink admitted")
	}
	for _, data := range [][]byte{nil, make([]byte, (8<<20)+1)} {
		if err := os.WriteFile(name, data, 0600); err != nil {
			t.Fatal(err)
		}
		if actual, err := readReviewedSums(name); err == nil || actual != nil {
			t.Fatal("invalid sum size admitted")
		}
	}
}
