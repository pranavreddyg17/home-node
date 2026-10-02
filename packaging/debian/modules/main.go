// Command modules records build information from the actual staged Go binaries.
// It produces unsigned development evidence, not license/vulnerability approval.
package main

import (
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"sort"
)

type binaryRecord struct {
	Path         string               `json:"path"`
	SHA256       string               `json:"sha256"`
	GoVersion    string               `json:"goVersion"`
	Main         debug.Module         `json:"main"`
	Dependencies []*debug.Module      `json:"dependencies"`
	Settings     []debug.BuildSetting `json:"settings"`
}

func inspect(root *os.Root, name string) (binaryRecord, error) {
	var zero binaryRecord
	file, err := root.Open(name)
	if err != nil {
		return zero, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return zero, err
	}
	if !stat.Mode().IsRegular() || stat.Size() < 1 || stat.Size() > 512<<20 {
		return zero, fmt.Errorf("invalid executable: %s", name)
	}
	info, err := buildinfo.Read(file)
	if err != nil {
		return zero, err
	}
	hash := sha256.New()
	count, err := io.Copy(hash, io.NewSectionReader(file, 0, stat.Size()))
	if err != nil || count != stat.Size() {
		return zero, fmt.Errorf("executable hashing failed: %s: %w", name, err)
	}
	after, err := file.Stat()
	if err != nil {
		return zero, err
	}
	if after.Size() != stat.Size() || !after.ModTime().Equal(stat.ModTime()) {
		return zero, fmt.Errorf("changed executable: %s", name)
	}
	sort.Slice(info.Deps, func(i, j int) bool { return info.Deps[i].Path < info.Deps[j].Path })
	return binaryRecord{Path: name, SHA256: hex.EncodeToString(hash.Sum(nil)), GoVersion: info.GoVersion, Main: info.Main, Dependencies: info.Deps, Settings: info.Settings}, nil
}

func run(args []string, output io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: modules STAGED_ROOT")
	}
	root, err := os.OpenRoot(args[0])
	if err != nil {
		return err
	}
	defer root.Close()
	records := []binaryRecord{}
	for _, name := range []string{"usr/bin/homenode", "usr/lib/homenode/homenode-supervisor", "usr/lib/homenode/homenode-transfer", "usr/lib/homenode/homenode-backup", "usr/lib/homenode/homenode-inspect", "usr/lib/homenode/guest/homenode-guest"} {
		record, err := inspect(root, name)
		if err != nil {
			return err
		}
		records = append(records, record)
	}
	return json.NewEncoder(output).Encode(struct {
		Schema       int            `json:"schema"`
		Completeness string         `json:"completeness"`
		Binaries     []binaryRecord `json:"binaries"`
	}{1, "incomplete", records})
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
