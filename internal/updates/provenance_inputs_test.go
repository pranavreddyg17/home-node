package updates

import (
	"strings"
	"testing"
)

func TestProvenanceInputsRequireReviewedParametersAndResolvedSource(t *testing.T) {
	hash, commit := strings.Repeat("ab", 32), strings.Repeat("cd", 20)
	params := `{"repository":"https://github.com/example/project","ref":"refs/tags/v1","count":1}`
	source := "git+https://github.com/example/project@refs/tags/v1"
	data := `{"_type":"https://in-toto.io/Statement/v1","subject":[{"digest":{"sha256":"` + hash + `"}}],"predicateType":"https://slsa.dev/provenance/v1","predicate":{"buildDefinition":{"buildType":"type","externalParameters":` + params + `,"resolvedDependencies":[{"uri":"` + source + `","digest":{"gitCommit":"` + commit + `"}}]},"runDetails":{"builder":{"id":"builder"}}}}`
	check := func(data, params, commit string) error {
		return ValidateProvenanceInputs([]byte(data), hash, "builder", "type", []byte(params), source, commit)
	}
	if err := check(data, params, commit); err != nil {
		t.Fatal(err)
	}
	for _, altered := range []string{
		strings.Replace(data, "refs/tags/v1", "refs/heads/main", 1),
		strings.Replace(data, commit, strings.Repeat("ef", 20), 1),
		strings.Replace(data, `"count":1`, `"count":"1"`, 1),
		strings.Replace(data, `"count":1`, `"count":1,"injected":true`, 1),
	} {
		if check(altered, params, commit) == nil {
			t.Fatal("unreviewed build inputs admitted")
		}
	}
	if check(data, `{"x":1,"x":2}`, commit) == nil || check(data, params, strings.ToUpper(commit)) == nil {
		t.Fatal("ambiguous policy admitted")
	}
}
