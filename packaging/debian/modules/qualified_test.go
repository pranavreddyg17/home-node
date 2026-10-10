package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCompleteCollectorReviewedCompiledSourceSums(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "main.go")
	if err := os.WriteFile(source, []byte("package main\nimport (\"fmt\"; \"golang.org/x/crypto/chacha20poly1305\")\nfunc main(){c,err:=chacha20poly1305.New(make([]byte,32));fmt.Println(c.NonceSize(),err)}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "build", "-o", filepath.Join(directory, "binary"), source)
	command.Env = append(os.Environ(), "GOENV=off", "GOWORK=off", "GOFLAGS=", "GOTOOLCHAIN=local", "GOPROXY=off", "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH, "CGO_ENABLED=0")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("compile pinned dependency fixture: %v: %s", err, output)
	}
	data, err := os.ReadFile(filepath.Join(directory, "binary"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"usr/bin/homenode", "usr/lib/homenode/homenode-supervisor", "usr/lib/homenode/homenode-transfer", "usr/lib/homenode/homenode-backup", "usr/lib/homenode/homenode-inspect", "usr/lib/homenode/homenode-gateway", "usr/lib/homenode/guest/homenode-guest"} {
		filename := filepath.Join(directory, name)
		if err := os.MkdirAll(filepath.Dir(filename), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	reviewed, err := os.ReadFile("../../../go.sum")
	if err != nil {
		t.Fatal(err)
	}
	sums := filepath.Join(directory, "reviewed.sum")
	if err := os.WriteFile(sums, reviewed, 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := run([]string{directory, sums}, &output); err != nil {
		t.Fatal("actual compiled source sums refused", err)
	}
	var record struct {
		SourceSumsVerified bool           `json:"sourceSumsVerified"`
		SourceSumsSHA256   string         `json:"sourceSumsSHA256"`
		Binaries           []binaryRecord `json:"binaries"`
	}
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(reviewed)
	if !record.SourceSumsVerified || record.SourceSumsSHA256 != hex.EncodeToString(digest[:]) || len(record.Binaries) != 7 || len(record.Binaries[0].Dependencies) == 0 {
		t.Fatal("compiled source sum claim differs from actual inputs")
	}
	dependency := record.Binaries[0].Dependencies[0]
	changed := strings.Replace(string(reviewed), dependency.Sum, "h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=", 1)
	if changed == string(reviewed) {
		t.Fatal("compiled source sum missing from reviewed input")
	}
	if err := os.WriteFile(sums, []byte(changed), 0600); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := run([]string{directory, sums}, &output); err == nil || output.Len() != 0 {
		t.Fatal("changed reviewed source sum exposed evidence", err)
	}
}
