package guest

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/guestproto"
	"github.com/pranavreddyg17/home-node/internal/state"
)

// Linux CI runs this inside its disposable runner. It validates the actual
// FFmpeg adapter against generated media, not the host isolation boundary.
func TestRealVideoConversion(t *testing.T) {
	if os.Getenv("HOMENODE_GUEST_INTEGRATION") != "1" {
		t.Skip("requires explicit disposable Linux media-test environment")
	}
	if runtime.GOOS != "linux" {
		t.Fatal("media integration requires Linux")
	}
	if _, err := os.Stat("/usr/bin/ffmpeg"); err != nil {
		t.Fatal(err)
	}
	inputPath := filepath.Join(t.TempDir(), "input.mp4")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/usr/bin/ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=320x240:rate=10", "-t", "1", "-c:v", "libx264", "-pix_fmt", "yuv420p", inputPath)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create fixture: %v %s", err, output)
	}
	data, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(t.TempDir(), "video", 8<<30)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	inputID, jobID := state.Random(), state.Random()
	for offset := 0; offset < len(data); {
		end := min(offset+guestproto.ChunkSize, len(data))
		chunk := data[offset:end]
		response := a.Handle(guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "upload", ObjectID: inputID, Size: int64(len(data)), Offset: int64(offset), Data: chunk, SHA256: checksum(chunk)})
		if response.Error != "" {
			t.Fatal(response)
		}
		offset = end
	}
	if response := a.Handle(guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "finalize", ObjectID: inputID, Size: int64(len(data)), SHA256: checksum(data)}); response.Error != "" {
		t.Fatal(response)
	}
	if response := a.Handle(guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "run", ObjectID: jobID, InputID: inputID, Preset: "mp4-720p"}); response.Error != "" {
		t.Fatal(response)
	}
	timeout := time.NewTimer(30 * time.Second)
	defer timeout.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-timeout.C:
			t.Fatal("conversion timed out")
		case <-ticker.C:
			result := a.Handle(guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "result", ObjectID: jobID})
			if result.State == "running" {
				continue
			}
			if result.State != "succeeded" || result.Size <= 0 || result.SHA256 == "" {
				t.Fatalf("conversion failed %+v", result)
			}
			chunk := a.Handle(guestproto.Request{Version: 1, RequestID: state.Random(), Operation: "download", ObjectID: jobID})
			if chunk.Error != "" || len(chunk.Data) == 0 {
				t.Fatal("missing output", chunk.Error)
			}
			return
		}
	}
}
