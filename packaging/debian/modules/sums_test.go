package main

import (
	"encoding/base64"
	"runtime/debug"
	"strings"
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

func TestCompiledModuleIdentityAmbiguityAndBounds(t *testing.T) {
	sum := "h1:" + base64.StdEncoding.EncodeToString(make([]byte, 32))
	data := []byte("fixture.example/dep v1.0.0 " + sum + "\n")
	cases := [][]*debug.Module{
		{nil},
		{{Path: "fixture.example/dep", Version: "v1.0.0", Sum: sum}, {Path: "fixture.example/dep", Version: "v1.0.0", Sum: sum}},
		{{Path: "fixture.example/dep", Version: "v1.0.0", Sum: "h1:" + strings.Repeat("A", 10000)}},
		{{Path: "fixture.example/dep", Replace: &debug.Module{Path: "fixture.example/dep", Version: "v1.0.0", Sum: sum, Replace: &debug.Module{Path: "foreign"}}}},
	}
	for _, dependencies := range cases {
		if err := verifyModuleSums([]binaryRecord{{Dependencies: dependencies}}, data); err == nil {
			t.Fatal("ambiguous or unbounded compiled identity accepted")
		}
	}
}

func TestPackageWideCompiledDependencyConsistency(t *testing.T) {
	sum := "h1:" + base64.StdEncoding.EncodeToString(make([]byte, 32))
	data := []byte("fixture.example/dep v1.0.0 " + sum + "\nfixture.example/dep v1.1.0 " + sum + "\nfixture.example/replacement v1.0.0 " + sum + "\n")
	records := []binaryRecord{
		{Dependencies: []*debug.Module{{Path: "fixture.example/dep", Version: "v1.0.0", Sum: sum}}},
		{Dependencies: []*debug.Module{{Path: "fixture.example/dep", Version: "v1.0.0", Sum: sum}}},
	}
	if err := verifyModuleSums(records, data); err != nil {
		t.Fatal("consistent packaged dependencies refused", err)
	}
	records[1].Dependencies[0].Version = "v1.1.0"
	if err := verifyModuleSums(records, data); err == nil {
		t.Fatal("mixed compiled dependency versions admitted")
	}
	records[1].Dependencies[0].Version = "v1.0.0"
	records[1].Dependencies[0].Replace = &debug.Module{Path: "fixture.example/replacement", Version: "v1.0.0", Sum: sum}
	if err := verifyModuleSums(records, data); err == nil {
		t.Fatal("mixed compiled replacement identities admitted")
	}
}
