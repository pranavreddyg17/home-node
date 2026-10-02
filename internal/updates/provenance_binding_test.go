package updates

import (
	"errors"
	"strings"
	"testing"
)

func TestProvenanceBindingRequiresIndependentSubjectBuilderAndType(t *testing.T) {
	hash := strings.Repeat("ab", 32)
	builder, buildType := "https://builder.example/v1", "https://build.example/v1"
	document := `{"_type":"https://in-toto.io/Statement/v1","subject":[{"digest":{"sha256":"` + hash + `"}}],"predicateType":"https://slsa.dev/provenance/v1","predicate":{"buildDefinition":{"buildType":"` + buildType + `","externalParameters":{}},"runDetails":{"builder":{"id":"` + builder + `"}}},"extension":true}`
	if err := ValidateProvenanceBinding([]byte(" \n"+document), hash, builder, buildType); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{
		strings.Replace(document, hash, strings.Repeat("cd", 32), 1),
		strings.Replace(document, builder, "https://other.example", 1),
		strings.Replace(document, buildType, "https://other.example", 1),
		strings.Replace(document, `"_type"`, `"_Type"`, 1),
		strings.Replace(document, `"externalParameters":{}`, `"externalParameters":null`, 1),
		strings.Replace(document, `"subject":[`, `"subject":[{"digest":{"sha256":"`+hash+`"}},`, 1),
		strings.Replace(document, `"extension":true`, `"extension":true,"extension":false`, 1),
	} {
		if err := ValidateProvenanceBinding([]byte(data), hash, builder, buildType); !errors.Is(err, ErrProvenanceBinding) {
			t.Fatal("wrong provenance admitted", err)
		}
	}
	for _, input := range []string{"", strings.ToUpper(hash), strings.Repeat("a", 1<<20)} {
		if ValidateProvenanceBinding([]byte(document), input, builder, buildType) == nil {
			t.Fatal("invalid expected digest admitted")
		}
	}
}
