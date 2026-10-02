package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestActualExecutableBuildInfoAndHash(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(executable)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "binary"), data, 0600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	record, err := inspect(root, "binary")
	if err != nil {
		t.Fatal(err)
	}
	expected := sha256.Sum256(data)
	if record.SHA256 != hex.EncodeToString(expected[:]) || record.GoVersion != runtime.Version() || record.Path != "binary" {
		t.Fatal("actual executable identity mismatch", record)
	}
	if err := os.WriteFile(filepath.Join(directory, "foreign"), []byte("not a Go executable"), 0600); err != nil {
		t.Fatal(err)
	}
	if record, err := inspect(root, "foreign"); err == nil || record.SHA256 != "" {
		t.Fatal("foreign bytes admitted", err)
	}
}

func TestCompiledModuleReplacementIsRetained(t *testing.T) {
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, "dep"), 0700); err != nil {
		t.Fatal(err)
	}
	sources := map[string]string{
		"go.mod":     "module fixture.example/app\n\ngo 1.26.0\nrequire fixture.example/dep v0.0.0\nreplace fixture.example/dep => ./dep\n",
		"main.go":    "package main\nimport (\"fmt\"; \"fixture.example/dep\")\nfunc main(){fmt.Println(dep.Value)}\n",
		"dep/go.mod": "module fixture.example/dep\n\ngo 1.26.0\n",
		"dep/dep.go": "package dep\nconst Value = 7\n",
	}
	for name, data := range sources {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	command := exec.Command("go", "build", "-o", "binary", ".")
	command.Dir = directory
	command.Env = append(os.Environ(), "GOENV=off", "GOWORK=off", "GOFLAGS=", "GOTOOLCHAIN=local", "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH, "CGO_ENABLED=0")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("compile replacement fixture: %v: %s", err, output)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	record, err := inspect(root, "binary")
	if err != nil {
		t.Fatal(err)
	}
	if record.Main.Path != "fixture.example/app" || len(record.Dependencies) != 1 {
		t.Fatal("compiled module inventory mismatch", record)
	}
	dep := record.Dependencies[0]
	if dep.Path != "fixture.example/dep" || dep.Version != "v0.0.0" || dep.Replace == nil || dep.Replace.Path != "./dep" || dep.Replace.Version != "(devel)" {
		t.Fatalf("module replacement lost: original=%+v replacement=%+v", dep, dep.Replace)
	}
}
