package main

import (
	"encoding/base64"
	"runtime/debug"
	"testing"
)

func TestCompiledModuleSourceSumBinding(t *testing.T) {
	sum := "h1:" + base64.StdEncoding.EncodeToString(make([]byte, 32))
	records := []binaryRecord{{Dependencies: []*debug.Module{{Path: "fixture.example/dep", Version: "v1.0.0", Sum: sum}}}}
	data := []byte("fixture.example/dep v1.0.0 " + sum + "\n")
	if err := verifyModuleSums(records, data); err != nil {
		t.Fatal(err)
	}
	if err := verifyModuleSums(records, []byte("fixture.example/dep v1.0.0 h1:foreign\n")); err == nil {
		t.Fatal("changed reviewed sum admitted")
	}
	if err := verifyModuleSums(records, append(append([]byte{}, data...), data...)); err == nil {
		t.Fatal("duplicate reviewed sums admitted")
	}
	records[0].Dependencies[0].Replace = &debug.Module{Path: "fixture.example/replacement", Version: "v2.0.0", Sum: sum}
	if err := verifyModuleSums(records, data); err == nil {
		t.Fatal("replacement authorized by original identity")
	}
	if err := verifyModuleSums(records, []byte("fixture.example/replacement v2.0.0 "+sum+"\n")); err != nil {
		t.Fatal(err)
	}
	records[0].Dependencies[0].Replace.Version = "(devel)"
	if err := verifyModuleSums(records, data); err == nil {
		t.Fatal("local replacement admitted source sum qualification")
	}
}
