package guest

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

func (a *Agent) modelReady() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:8080/health", nil)
	if err != nil {
		return false
	}
	response, err := a.readinessClient.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	if err != nil || len(data) > 4096 {
		return false
	}
	return readyModelResponse(data)
}
func readyModelResponse(data []byte) bool {
	if len(data) > 4096 {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return false
	}
	seen := map[string]bool{}
	ready := false
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return false
		}
		key, ok := token.(string)
		if !ok || seen[key] {
			return false
		}
		seen[key] = true
		switch key {
		case "status":
			var status string
			if d.Decode(&status) != nil {
				return false
			}
			ready = status == "ok"
		case "slots_idle", "slots_processing":
			var n int
			if d.Decode(&n) != nil || n < 0 {
				return false
			}
		default:
			return false
		}
	}
	token, err = d.Token()
	if err != nil || token != json.Delim('}') || d.Decode(new(any)) != io.EOF {
		return false
	}
	return ready
}
