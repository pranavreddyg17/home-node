package guest

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"syscall"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

// Allows the maximum 32 KiB text even when JSON escapes expand each byte.
const maxTaskJournalBytes = 256 << 10

var errTaskJournal = errors.New("invalid task journal")

func (a *Agent) recoverTasks() error {
	directory, err := a.root.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	for {
		entries, err := directory.ReadDir(128)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".task.json") {
				continue
			}
			id := strings.TrimSuffix(entry.Name(), ".task.json")
			if !guestproto.ValidID(id) || len(a.tasks) >= 1000 {
				return errTaskJournal
			}
			t, err := readTaskJournal(a.root, entry.Name(), a.kind)
			if err != nil {
				return err
			}
			a.tasks[id] = &t
		}
		if errors.Is(err, io.EOF) {
			break
		}
	}
	// Validate every record before rewriting any interrupted intent.
	for id, t := range a.tasks {
		if t.State == "running" {
			t.State = "interrupted"
			if err = a.save(id, t); err != nil {
				return err
			}
		}
	}
	return nil
}
func readTaskJournal(root *os.Root, name, kind string) (task, error) {
	var t task
	f, err := root.OpenFile(name, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return t, errTaskJournal
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || info.Size() > maxTaskJournalBytes {
		return t, errTaskJournal
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); !ok || int(stat.Uid) != os.Geteuid() {
		return t, errTaskJournal
	}
	data, err := io.ReadAll(io.LimitReader(f, maxTaskJournalBytes+1))
	if err != nil || len(data) > maxTaskJournalBytes {
		return t, errTaskJournal
	}
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return t, errTaskJournal
	}
	seen := map[string]bool{}
	for d.More() {
		token, err = d.Token()
		if err != nil {
			return t, errTaskJournal
		}
		key, ok := token.(string)
		if !ok || seen[key] {
			return t, errTaskJournal
		}
		seen[key] = true
		var target any
		switch key {
		case "state":
			target = &t.State
		case "inputId":
			target = &t.InputID
		case "preset":
			target = &t.Preset
		case "promptHash":
			target = &t.PromptHash
		case "text":
			target = &t.Text
		case "size":
			target = &t.Size
		case "sha256":
			target = &t.SHA256
		default:
			return t, errTaskJournal
		}
		if d.Decode(target) != nil {
			return t, errTaskJournal
		}
	}
	token, err = d.Token()
	if err != nil || token != json.Delim('}') || d.Decode(new(any)) != io.EOF || !validRecoveredTask(t, kind) {
		return t, errTaskJournal
	}
	return t, nil
}
func validTaskDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	raw, err := hex.DecodeString(value)
	return err == nil && hex.EncodeToString(raw) == value
}
func validRecoveredTask(t task, kind string) bool {
	switch t.State {
	case "running", "succeeded", "failed", "cancelled", "interrupted":
	default:
		return false
	}
	if !validTaskDigest(t.PromptHash) || t.Size < 0 || t.Size > maxOutput || (t.SHA256 != "" && !validTaskDigest(t.SHA256)) || len(t.Text) > 32<<10 {
		return false
	}
	if kind == "video" {
		if !guestproto.ValidID(t.InputID) || (t.Preset != "mp4-720p" && t.Preset != "mp4-1080p") || t.Text != "" {
			return false
		}
		if t.State == "succeeded" && (t.Size == 0 || t.Size >= maxOutput || !validTaskDigest(t.SHA256)) {
			return false
		}
		return true
	}
	if kind == "ai" {
		return t.Preset == "" && (t.InputID == "" || guestproto.ValidID(t.InputID)) && t.Size == 0 && t.SHA256 == "" && (t.State == "succeeded" || t.Text == "")
	}
	return false
}
