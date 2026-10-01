package control

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMaintenanceRequestStrictOwnershipFields(t *testing.T) {
	id := strings.Repeat("a", 24)
	valid := `{"version":1,"token":"` + id + `","jobId":"` + id + `","deviceId":"` + id + `"}`
	for _, scenario := range []string{"valid", "duplicate", "alias", "null", "unknown", "missing", "short", "trailing", "oversized", "invalid-utf8"} {
		t.Run(scenario, func(t *testing.T) {
			data := valid
			switch scenario {
			case "duplicate":
				data = strings.Replace(data, `"version":1`, `"version":1,"version":1`, 1)
			case "alias":
				data = strings.Replace(data, `"version":1`, `"version":1,"vers\u0069on":1`, 1)
			case "null":
				data = strings.Replace(data, `"version":1`, `"version":null`, 1)
			case "unknown":
				data = strings.Replace(data, `"version":1`, `"version":1,"command":"start"`, 1)
			case "missing":
				data = strings.Replace(data, `"version":1,`, "", 1)
			case "short":
				data = strings.Replace(data, id, "short", 1)
			case "trailing":
				data += "{}"
			case "oversized":
				data += strings.Repeat(" ", 512)
			case "invalid-utf8":
				data += string([]byte{255})
			}
			_, ok := decodeMaintenanceRequest([]byte(data))
			if ok != (scenario == "valid") {
				t.Fatal("unexpected ownership request admission", ok)
			}
		})
	}
}

func TestMaintenanceHandlerCannotUseHTTPPeerClaims(t *testing.T) {
	server := testServer(t)
	request := httptest.NewRequest("POST", "/v1/maintenance/drain", strings.NewReader(`{"version":1}`))
	request.Header.Set("X-Peer-UID", "1003")
	response := httptest.NewRecorder()
	server.MaintenanceHandler(1001, 1003).ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatal("HTTP peer claim accepted", response.Code)
	}
	var count int
	if err := server.Store.DB.QueryRow("SELECT count(*) FROM settings WHERE key LIKE 'host.maintenance%'").Scan(&count); err != nil || count != 0 {
		t.Fatal("refused request acquired maintenance", count, err)
	}
}
