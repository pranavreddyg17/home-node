package updates

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/theupdateframework/go-tuf/v2/metadata"
)

func TestReleaseEvidenceRequiresExactTargetBytesAndJSON(t *testing.T) {
	for _, scenario := range []string{"valid", "corrupt", "short", "invalid-json", "traversal", "missing-sha256", "oversized", "duplicate", "escaped-duplicate", "nested-duplicate", "scalar", "invalid-utf8"} {
		t.Run(scenario, func(t *testing.T) {
			payload := []byte(`{"build":"fixture"}`)
			if scenario == "invalid-json" {
				payload = []byte(`not a JSON document`)
			}
			switch scenario {
			case "duplicate":
				payload = []byte(`{"build":"one","build":"two"}`)
			case "escaped-duplicate":
				payload = []byte(`{"build":"one","\u0062uild":"two"}`)
			case "nested-duplicate":
				payload = []byte(`{"nested":[{"build":"one","build":"two"}]}`)
			case "scalar":
				payload = []byte(`true`)
			case "invalid-utf8":
				payload = []byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}
			}
			sum := sha256.Sum256(payload)
			target := &metadata.TargetFiles{Path: "release/provenance.json", Length: int64(len(payload)), Hashes: metadata.Hashes{"sha256": sum[:]}}
			switch scenario {
			case "traversal":
				target.Path = "release/../provenance.json"
			case "missing-sha256":
				target.Hashes = nil
			case "oversized":
				target.Length = 8<<20 + 1
			}
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				expected := "/targets/release/" + hex.EncodeToString(sum[:]) + ".provenance.json"
				if r.URL.Path != expected {
					t.Error("wrong evidence path", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				data := append([]byte(nil), payload...)
				switch scenario {
				case "corrupt":
					data[0] ^= 1
				case "short":
					data = data[:len(data)-1]
				}
				_, _ = w.Write(data)
			}))
			defer server.Close()
			fetcher, err := newMetadataFetcher(context.Background(), server.URL+"/targets")
			if err != nil {
				t.Fatal(err)
			}
			fetcher.client.Transport = server.Client().Transport
			data, err := downloadReleaseEvidence(fetcher, target, true)
			if scenario == "valid" {
				if err != nil || string(data) != string(payload) {
					t.Fatal(string(data), err)
				}
			} else if err == nil || data != nil {
				t.Fatal("invalid evidence accepted", string(data), err)
			}
		})
	}
}
