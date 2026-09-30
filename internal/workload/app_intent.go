package workload

import (
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

func parseAppIntent(payload, kind string) (appIntent, error) {
	var intent appIntent
	if len(payload) > 1024 || !utf8.ValidString(payload) {
		return intent, ErrConflict
	}
	d := json.NewDecoder(strings.NewReader(payload))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return intent, ErrConflict
	}
	seen := map[string]bool{}
	for d.More() {
		token, err = d.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] {
			return intent, ErrConflict
		}
		seen[key] = true
		var target any
		switch key {
		case "revision":
			target = &intent.Revision
		case "workload":
			target = &intent.Workload
		case "action":
			target = &intent.Action
		case "instanceId":
			target = &intent.InstanceID
		case "cooperativeStop":
			target = &intent.CooperativeStop
		default:
			return intent, ErrConflict
		}
		var raw json.RawMessage
		if d.Decode(&raw) != nil || string(raw) == "null" || json.Unmarshal(raw, target) != nil {
			return intent, ErrConflict
		}
	}
	token, err = d.Token()
	if err != nil || token != json.Delim('}') {
		return intent, ErrConflict
	}
	if _, err = d.Token(); err != io.EOF {
		return intent, ErrConflict
	}
	if !seen["revision"] || !seen["workload"] || !seen["action"] || !seen["instanceId"] || intent.Revision < 1 || intent.Revision > 1<<53 || !guestproto.ValidID(intent.InstanceID) || (intent.Workload != "files" && intent.Workload != "ai") || (intent.Action != "start" && intent.Action != "stop") || kind != "app."+intent.Action || intent.CooperativeStop && intent.Action != "stop" {
		return intent, ErrConflict
	}
	return intent, nil
}
