package updates

import (
	"strings"
	"testing"
)

func TestEvidenceJSONRejectsAmbiguityAndUnboundedStructure(t *testing.T) {
	for _, data := range []string{`{}`, `{"components":[],"nested":{"field":null},"value":12345678901234567890}`} {
		if !validEvidenceJSON([]byte(data)) {
			t.Fatal("bounded object refused", data)
		}
	}
	for _, data := range []string{`null`, `true`, `[]`, `{"x":1,"x":2}`, `{"x":1,"\u0078":2}`, `{"nested":[{"x":1,"x":2}]}`, `{} {}`, `{"x":`, `{"x":` + strings.Repeat("[", 65) + `0` + strings.Repeat("]", 65) + `}`, `{"x":[` + strings.Repeat("0,", 65536) + `0]}`, strings.Repeat(" ", 8<<20) + `{}`} {
		if validEvidenceJSON([]byte(data)) {
			t.Fatal("ambiguous or unbounded evidence admitted")
		}
	}
}

func TestEvidenceJSONRejectsTrailingDocumentBeforeMaterializingIt(t *testing.T) {
	data := []byte(`{} {"components":[` + strings.Repeat(`{"x":1},`, 10000) + `{}]}`)
	allocations := testing.AllocsPerRun(5, func() {
		if validEvidenceJSON(data) {
			t.Fatal("trailing document accepted")
		}
	})
	if allocations > 200 {
		t.Fatal("trailing document materialized outside token budget", allocations)
	}
}
