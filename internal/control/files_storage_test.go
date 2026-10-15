package control

import (
	"encoding/json"
	"testing"

	"github.com/pranavreddyg17/home-node/internal/workload"
)

func TestFilesStorageRequiresFilesCapability(t *testing.T) {
	url := "http://localhost:8787/api/v1/files/storage"
	for _, test := range []struct {
		capabilities string
		status       int
	}{
		{"", 401},
		{`["ai"]`, 403},
		{`["files"]`, 200},
	} {
		s := testServer(t)
		token := ""
		if test.capabilities != "" {
			token = seedSession(t, s, test.capabilities)
		}
		response := request(s, "GET", url, "", token, "")
		if response.Code != test.status {
			t.Fatal("storage authority", response.Code, response.Body.String())
		}
		if test.status == 200 {
			var usage workload.FilesStorage
			if err := json.Unmarshal(response.Body.Bytes(), &usage); err != nil || usage.QuotaBytes != workload.StorageQuota || usage.AvailableBytes != usage.QuotaBytes {
				t.Fatal(usage, err)
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("storage metadata may be cached")
			}
		}
	}
}

func TestPendingTransfersRequireFilesCapability(t *testing.T) {
	for _, test := range []struct {
		capabilities string
		status       int
	}{
		{"", 401}, {`["ai"]`, 403}, {`["files"]`, 200},
	} {
		s := testServer(t)
		token := ""
		if test.capabilities != "" {
			token = seedSession(t, s, test.capabilities)
		}
		response := request(s, "GET", "http://localhost:8787/api/v1/transfers", "", token, "")
		if response.Code != test.status {
			t.Fatal("pending upload authority", response.Code, response.Body.String())
		}
		if test.status == 200 && (response.Body.String() != "[]\n" || response.Header().Get("Cache-Control") != "no-store") {
			t.Fatal("empty pending uploads must be a private JSON list", response.Body.String())
		}
	}
}
