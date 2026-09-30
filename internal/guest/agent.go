// Package guest runs inside an isolated workload VM. It never runs in the
// control process. Media and model parsing stay on this side of the boundary.
package guest

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
)

const maxOutput int64 = 4 << 30

var errQuota = errors.New("storage quota exceeded")

type task struct {
	State      string `json:"state"`
	InputID    string `json:"inputId,omitempty"`
	Preset     string `json:"preset,omitempty"`
	PromptHash string `json:"promptHash,omitempty"`
	Text       string `json:"text,omitempty"`
	Size       int64  `json:"size,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	cancel     context.CancelFunc
	active     bool // Cancellation is visible before the worker has joined.
}
type Agent struct {
	root            *os.Root
	kind            string
	quota           int64
	mu              sync.Mutex
	tasks           map[string]*task
	client          *http.Client
	readinessClient *http.Client
	workers         sync.WaitGroup
	closeOnce       sync.Once
	closed          bool
	closeErr        error
	workerErr       error
}

func New(directory, kind string, quota int64) (*Agent, error) {
	if kind != "files" && kind != "video" && kind != "ai" || quota < 1<<30 || quota > 512<<30 {
		return nil, errors.New("invalid guest profile")
	}
	if err := os.Mkdir(directory, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	before, err := os.Lstat(directory)
	if err != nil || !before.IsDir() || before.Mode().Perm() != 0700 {
		return nil, errors.New("guest data root is not private")
	}
	owner, ok := before.Sys().(*syscall.Stat_t)
	if !ok || int(owner.Uid) != os.Geteuid() {
		return nil, errors.New("guest data root ownership mismatch")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(before, opened) || opened.Mode().Perm() != 0700 {
		root.Close()
		return nil, errors.New("guest data root changed during admission")
	}
	a := &Agent{root: root, kind: kind, quota: quota, tasks: map[string]*task{}, client: &http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect denied") }, Transport: &http.Transport{Proxy: nil, MaxConnsPerHost: 1, ResponseHeaderTimeout: 15 * time.Second}}}
	a.readinessClient = &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect denied") }, Transport: &http.Transport{Proxy: nil, MaxConnsPerHost: 1, ResponseHeaderTimeout: time.Second}}
	if err = a.recoverTasks(); err != nil {
		root.Close()
		return nil, err
	}
	return a, nil
}
func (a *Agent) Close() error {
	a.closeOnce.Do(func() {
		a.mu.Lock()
		a.closed = true
		for _, t := range a.tasks {
			if t.cancel != nil {
				t.cancel()
			}
		}
		a.mu.Unlock()
		a.workers.Wait()
		a.client.CloseIdleConnections()
		a.readinessClient.CloseIdleConnections()
		a.closeErr = errors.Join(a.workerErr, a.sync(), a.root.Close())
	})
	return a.closeErr
}
func (a *Agent) Serve(stream io.ReadWriter) error {
	for {
		var r guestproto.Request
		if err := guestproto.Read(stream, &r); err != nil {
			return err
		}
		response := a.Handle(r)
		if err := guestproto.Write(stream, response); err != nil {
			return err
		}
	}
}
func (a *Agent) Handle(r guestproto.Request) guestproto.Response {
	response := guestproto.Response{Version: 1, RequestID: r.RequestID}
	if err := guestproto.Validate(r); err != nil {
		response.Error = "INVALID_REQUEST"
		return response
	}
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		response.Error = "WORKLOAD_UNAVAILABLE"
		return response
	}
	if r.Operation == "health" && a.kind == "ai" {
		a.mu.Unlock()
		response.State = "starting"
		if a.modelReady() {
			response.State = "ready"
		}
		a.mu.Lock()
		if a.closed {
			response.State = ""
			response.Error = "WORKLOAD_UNAVAILABLE"
		}
		a.mu.Unlock()
		return response
	}
	defer a.mu.Unlock()
	var err error
	switch r.Operation {
	case "health":
		response.State = "ready"
	case "upload":
		response.Offset, err = a.upload(r)
	case "finalize":
		response.Size, response.SHA256, err = a.finalize(r)
	case "stat":
		response.Size, response.SHA256, err = a.stat(r.ObjectID)
	case "download":
		response.Data, response.SHA256, err = a.download(r)
		response.Offset = r.Offset + int64(len(response.Data))
	case "delete":
		for id, t := range a.tasks {
			if (t.active || t.State == "running") && (t.InputID == r.ObjectID || id == r.ObjectID) {
				response.Error = "OBJECT_BUSY"
				return response
			}
		}
		for _, suffix := range []string{".part", ".blob", ".task.json", ".task.tmp", ".working"} {
			if e := a.root.Remove(r.ObjectID + suffix); e != nil && !errors.Is(e, os.ErrNotExist) {
				err = e
			}
		}
		delete(a.tasks, r.ObjectID)
		if err == nil {
			err = a.sync()
		}
	case "run", "generate":
		err = a.start(r)
		if err == nil {
			response.State = a.tasks[r.ObjectID].State
		}
	case "cancel":
		t, ok := a.tasks[r.ObjectID]
		if !ok {
			err = os.ErrNotExist
			break
		}
		if t.State == "running" {
			t.cancel()
			t.State = "cancelled"
			err = a.save(r.ObjectID, t)
		}
		response.State = t.State
	case "result":
		t, ok := a.tasks[r.ObjectID]
		if !ok {
			err = os.ErrNotExist
			break
		}
		response.State = t.State
		response.Text = t.Text
		response.Size = t.Size
		response.SHA256 = t.SHA256
	}
	if err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			response.Error = "NOT_FOUND"
		case errors.Is(err, errQuota):
			response.Error = "CAPACITY_UNAVAILABLE"
		default:
			response.Error = "OPERATION_FAILED"
		}
	}
	return response
}
func (a *Agent) used() (int64, error) {
	entries, err := fs.ReadDir(a.root.FS(), ".")
	if err != nil {
		return 0, err
	}
	var used int64
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return 0, err
		}
		if !info.Mode().IsRegular() {
			return 0, errors.New("unexpected data entry")
		}
		used += info.Size()
		if used > a.quota {
			return used, errQuota
		}
	}
	return used, nil
}
func checksum(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func (a *Agent) upload(r guestproto.Request) (int64, error) {
	if len(r.Data) == 0 || checksum(r.Data) != r.SHA256 || r.Size < r.Offset+int64(len(r.Data)) || r.Size > a.quota {
		return 0, guestproto.ErrProtocol
	}
	if _, err := a.root.Stat(r.ObjectID + ".blob"); err == nil {
		return 0, errors.New("object already finalized")
	}
	file, err := a.root.OpenFile(r.ObjectID+".part", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return 0, err
	}
	if r.Offset < info.Size() {
		if r.Offset+int64(len(r.Data)) > info.Size() {
			return info.Size(), errors.New("partial chunk requires reconciliation")
		}
		existing := make([]byte, len(r.Data))
		if _, err = file.ReadAt(existing, r.Offset); err != nil {
			return info.Size(), err
		}
		if !bytes.Equal(existing, r.Data) {
			return info.Size(), errors.New("chunk conflict")
		}
		return info.Size(), nil
	}
	if r.Offset != info.Size() {
		return info.Size(), errors.New("offset conflict")
	}
	used, err := a.used()
	if err != nil {
		return info.Size(), err
	}
	if used+int64(len(r.Data)) > a.quota-(64<<20) {
		return info.Size(), errQuota
	}
	if _, err = file.WriteAt(r.Data, r.Offset); err != nil {
		return info.Size(), err
	}
	if err = file.Sync(); err != nil {
		return info.Size(), err
	}
	return r.Offset + int64(len(r.Data)), nil
}
func (a *Agent) hash(name string) (int64, string, error) {
	file, err := a.root.Open(name)
	if err != nil {
		return 0, "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return 0, "", errors.New("invalid object")
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(file, a.quota+1))
	if err != nil {
		return 0, "", err
	}
	if n > a.quota {
		return 0, "", errQuota
	}
	return n, hex.EncodeToString(hash.Sum(nil)), nil
}
func (a *Agent) stat(id string) (int64, string, error) {
	if info, err := a.root.Stat(id + ".part"); err == nil {
		return info.Size(), "", nil
	}
	return a.hash(id + ".blob")
}
func (a *Agent) finalize(r guestproto.Request) (int64, string, error) {
	size, hash, err := a.hash(r.ObjectID + ".blob")
	if err == nil {
		if size != r.Size || hash != r.SHA256 {
			return 0, "", errors.New("finalize conflict")
		}
		return size, hash, nil
	}
	if r.Size == 0 && r.SHA256 == checksum(nil) {
		f, e := a.root.OpenFile(r.ObjectID+".part", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e == nil {
			e = f.Sync()
			_ = f.Close()
		}
		if e != nil && !errors.Is(e, os.ErrExist) {
			return 0, "", e
		}
	}
	size, hash, err = a.hash(r.ObjectID + ".part")
	if err != nil {
		return 0, "", err
	}
	if size != r.Size || hash != r.SHA256 {
		return 0, "", errors.New("object integrity failure")
	}
	if err = a.root.Rename(r.ObjectID+".part", r.ObjectID+".blob"); err != nil {
		return 0, "", err
	}
	return size, hash, a.sync()
}
func (a *Agent) download(r guestproto.Request) ([]byte, string, error) {
	f, err := a.root.Open(r.ObjectID + ".blob")
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, "", err
	}
	if r.Offset > info.Size() {
		return nil, "", guestproto.ErrProtocol
	}
	size := min(int64(guestproto.ChunkSize), info.Size()-r.Offset)
	data := make([]byte, size)
	if _, err = f.ReadAt(data, r.Offset); err != nil && err != io.EOF {
		return nil, "", err
	}
	return data, checksum(data), nil
}
func (a *Agent) sync() error {
	dir, err := a.root.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
func (a *Agent) save(id string, t *task) error {
	data, err := json.Marshal(t)
	if len(data) > maxTaskJournalBytes {
		return errors.New("invalid task journal")
	}
	if err != nil {
		return err
	}
	f, err := a.root.OpenFile(id+".task.tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = a.root.Rename(id+".task.tmp", id+".task.json"); err != nil {
		return err
	}
	return a.sync()
}
func (a *Agent) start(r guestproto.Request) error {
	if r.Operation == "run" && a.kind != "video" || r.Operation == "generate" && a.kind != "ai" {
		return errors.New("wrong workload")
	}
	if old, ok := a.tasks[r.ObjectID]; ok {
		if old.InputID != r.InputID || old.Preset != r.Preset || old.PromptHash != promptDigest(r) {
			return errors.New("request conflict")
		}
		return nil
	}
	if len(a.tasks) >= 1000 {
		return errQuota
	}
	for _, t := range a.tasks {
		if t.active || t.State == "running" {
			return errors.New("workload busy")
		}
	}
	used, err := a.used()
	if err != nil {
		return err
	}
	if r.Operation == "run" && used+maxOutput > a.quota-(64<<20) {
		return errQuota
	}
	duration := 30 * time.Minute
	if r.Operation == "generate" {
		duration = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	t := &task{State: "running", InputID: r.InputID, Preset: r.Preset, PromptHash: promptDigest(r), cancel: cancel, active: true}
	if err = a.save(r.ObjectID, t); err != nil {
		cancel()
		return err
	}
	a.tasks[r.ObjectID] = t
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		defer cancel()
		var text string
		var err error
		if r.Operation == "run" {
			err = a.convert(ctx, r)
		} else {
			text, err = a.generate(ctx, r)
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		defer func() { t.active = false }()
		if t.State == "cancelled" {
			_ = a.root.Remove(r.ObjectID + ".working")
			return
		}
		if a.closed {
			t.State = "interrupted"
			_ = a.root.Remove(r.ObjectID + ".working")
		} else if err != nil {
			t.State = "failed"
			_ = a.root.Remove(r.ObjectID + ".working")
		} else {
			t.State = "succeeded"
			t.Text = text
			if r.Operation == "run" {
				t.Size, t.SHA256, err = a.hash(r.ObjectID + ".working")
				if err != nil || t.Size == 0 || t.Size >= maxOutput {
					t.State = "failed"
				} else if err = a.root.Rename(r.ObjectID+".working", r.ObjectID+".blob"); err != nil {
					t.State = "failed"
				}
			}
		}
		if err = a.save(r.ObjectID, t); err != nil {
			t.State = "interrupted"
			a.workerErr = errors.Join(a.workerErr, err)
		}
	}()
	return nil
}
func (a *Agent) convert(ctx context.Context, r guestproto.Request) error {
	input, err := a.root.Open(r.InputID + ".blob")
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := a.root.OpenFile(r.ObjectID+".working", os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer output.Close()
	height := "720"
	if r.Preset == "mp4-1080p" {
		height = "1080"
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-max_alloc", "268435456", "-protocol_whitelist", "file,pipe", "-threads", "2", "-filter_threads", "2", "-i", "/proc/self/fd/3", "-map", "0:v:0?", "-map", "0:a:0?", "-vf", "scale=-2:min(ih\\,"+height+")", "-c:v", "libx264", "-threads", "2", "-preset", "veryfast", "-crf", "28", "-c:a", "aac", "-b:a", "128k", "-movflags", "+faststart", "-fs", "4294967296", "-f", "mp4", "/proc/self/fd/4")
	cmd.ExtraFiles = []*os.File{input, output}
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C"}
	cmd.WaitDelay = 5 * time.Second
	if err = cmd.Run(); err != nil {
		return err
	}
	return output.Sync()
}
func (a *Agent) generate(ctx context.Context, r guestproto.Request) (string, error) {
	messages := r.Messages
	if r.InputID != "" {
		file, err := a.root.Open(r.InputID + ".blob")
		if err != nil {
			return "", err
		}
		data, err := io.ReadAll(io.LimitReader(file, (16<<10)+1))
		_ = file.Close()
		if err != nil || len(data) > 16<<10 {
			return "", guestproto.ErrProtocol
		}
		if err = json.Unmarshal(data, &messages); err != nil {
			return "", guestproto.ErrProtocol
		}
		check := r
		check.InputID = ""
		check.Messages = messages
		if err = guestproto.Validate(check); err != nil {
			return "", err
		}
	}
	if len(messages) == 0 {
		messages = []guestproto.Message{{Role: "user", Content: r.Prompt}}
	}
	body, _ := json.Marshal(map[string]any{"messages": messages, "max_tokens": 256, "stream": true, "cache_prompt": false})
	req, err := http.NewRequestWithContext(ctx, "POST", "http://127.0.0.1:8080/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return "", errors.New("inference failed")
	}
	scan := bufio.NewScanner(io.LimitReader(response.Body, 1<<20))
	scan.Buffer(make([]byte, 4096), 64<<10)
	text := ""
	stopped := false
	for scan.Scan() {
		line := scan.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		value := strings.TrimPrefix(line, "data: ")
		if value == "[DONE]" {
			break
		}
		var token struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(value), &token) != nil || len(token.Choices) > 1 {
			return "", errors.New("invalid model output")
		}
		if len(token.Choices) == 0 {
			continue
		}
		content := token.Choices[0].Delta.Content
		if token.Choices[0].FinishReason != nil {
			stopped = true
		}

		if len(text)+len(content) > 32<<10 {
			return "", errors.New("model output limit")
		}
		text += content
		a.mu.Lock()
		if task, ok := a.tasks[r.ObjectID]; ok && task.State == "running" {
			task.Text = text
		}
		a.mu.Unlock()
		if stopped {
			stopped = true
			break
		}
	}
	if err = scan.Err(); err != nil {
		return "", err
	}
	if !stopped {
		return "", errors.New("incomplete model output")
	}
	return text, nil
}

func promptDigest(r guestproto.Request) string {
	data, _ := json.Marshal(struct {
		Prompt   string
		Messages []guestproto.Message
	}{r.Prompt, r.Messages})
	return checksum(data)
}
