//go:build linux

package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

type nativeRevocableDownload struct {
	response *http.Response
	ctx      context.Context
	cancel   context.CancelFunc
	size     int64
}

// The 1 GiB object exceeds bounded gateway/controller and kernel send buffers.
// Pause reads after a verified prefix so revocation crosses an active data path.
func startNativeRevocableDownload(t *testing.T, paired *http.Client, origin string, file *nativeHTTPSFilesResult) *nativeRevocableDownload {
	t.Helper()
	if file.Size != 1<<30 {
		t.Fatal("active native revocation requires the large Files object")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	req, err := http.NewRequestWithContext(ctx, "GET", origin+"/api/v1/files/"+file.ID+"/download", nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	req.Header.Set("Origin", origin)
	// Avoid the ordinary protocol client's five-second timeout: this stream
	// has its own bounded context, covering paused reads and revocation.
	client := &http.Client{Transport: paired.Transport, Jar: paired.Jar}
	response, err := client.Do(req)
	if err != nil {
		cancel()
		t.Fatal("paired native download failed to open", err)
	}
	stream := &nativeRevocableDownload{response: response, ctx: ctx, cancel: cancel, size: file.Size}
	if response.StatusCode != 200 || response.TLS == nil || len(response.TLS.VerifiedChains) == 0 || response.ContentLength != file.Size || response.Header.Get("ETag") != `"`+file.SHA256+`"` || response.Header.Get("X-Content-SHA256") != file.SHA256 {
		stream.Close()
		t.Fatal("paired native download lacked verified identity and length")
	}
	if n, err := io.CopyN(io.Discard, response.Body, 16<<10); err != nil || n != 16<<10 {
		stream.Close()
		t.Fatal("paired native download failed before revocation", n, err)
	}
	return stream
}

func (s *nativeRevocableDownload) Close() {
	s.response.Body.Close()
	s.cancel()
}

func (s *nativeRevocableDownload) ExpectRevoked(t *testing.T) {
	t.Helper()
	remaining, err := io.Copy(io.Discard, io.LimitReader(s.response.Body, s.size-(16<<10)+1))
	if !errors.Is(err, io.ErrUnexpectedEOF) || remaining+(16<<10) >= s.size || s.ctx.Err() != nil {
		t.Fatal("revoked native download did not truncate before its deadline", remaining, err, s.ctx.Err())
	}
	s.Close()
	t.Logf("native HTTPS paired-device revocation truncated active guest download after %d of %d bytes; owner integrity verification follows", remaining+(16<<10), s.size)
}
