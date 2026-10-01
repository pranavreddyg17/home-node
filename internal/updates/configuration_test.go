package updates

import "testing"

func TestRepositoryConfigurationScopesAndObservedSchema(t *testing.T) {
	valid := RepositoryConfiguration{Schema: 1, MetadataURL: "https://updates.example/metadata/", TargetsURL: "https://updates.example/targets/", MinimumSequence: 5, MinimumCatalogVersion: 3}
	policy, err := valid.Policy(4)
	if err != nil || policy != (ReleasePolicy{MinimumSequence: 5, MinimumCatalogVersion: 3, CurrentStateSchema: 4}) {
		t.Fatal(policy, err)
	}
	for _, scenario := range []string{"schema", "sequence", "catalog", "http", "different-host", "same-scope", "credentials", "query", "traversal"} {
		t.Run(scenario, func(t *testing.T) {
			c := valid
			switch scenario {
			case "schema":
				c.Schema = 2
			case "sequence":
				c.MinimumSequence = 0
			case "catalog":
				c.MinimumCatalogVersion = 0
			case "http":
				c.MetadataURL = "http://updates.example/metadata/"
			case "different-host":
				c.TargetsURL = "https://other.example/targets/"
			case "same-scope":
				c.TargetsURL = c.MetadataURL
			case "credentials":
				c.MetadataURL = "https://user:password@updates.example/metadata/"
			case "query":
				c.TargetsURL += "?alternate=1"
			case "traversal":
				c.TargetsURL = "https://updates.example/../targets/"
			}
			if c.Validate() == nil {
				t.Fatal("unsafe repository accepted")
			}
		})
	}
	for _, schema := range []int{0, 1025} {
		if _, err := valid.Policy(schema); err == nil {
			t.Fatal("invalid observed schema accepted")
		}
	}
}
