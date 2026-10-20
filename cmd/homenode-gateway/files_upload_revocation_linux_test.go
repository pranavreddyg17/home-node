//go:build linux

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptrace"
	"sync"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/workload"
)

type nativeUploadResult struct {
	response *http.Response
	err      error
}
type nativeRevocableUpload struct {
	id        string
	cancel    context.CancelFunc
	ctx       context.Context
	release   chan struct{}
	once      sync.Once
	done      chan nativeUploadResult
	reader    *io.PipeReader
	transport *http.Transport
}

func startNativeRevocableUpload(t *testing.T, paired *http.Client, origin string) *nativeRevocableUpload {
	t.Helper()
	data := make([]byte, 32<<10)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(data)
	body, _ := json.Marshal(map[string]any{"name": "revoked-upload.bin", "size": len(data), "sha256": hex.EncodeToString(hash[:])})
	req, _ := http.NewRequest("POST", origin+"/api/v1/transfers", bytes.NewReader(body))
	req.Header.Set("Origin", origin)
	req.Header.Set("Content-Type", "application/json")
	created, err := paired.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	encoded, readErr := io.ReadAll(io.LimitReader(created.Body, 4097))
	created.Body.Close()
	var transfer workload.Transfer
	if readErr != nil || len(encoded) > 4096 || created.StatusCode != 201 || created.TLS == nil || len(created.TLS.VerifiedChains) == 0 || json.Unmarshal(encoded, &transfer) != nil || transfer.ID == "" || transfer.Offset != 0 {
		t.Fatal("paired upload creation failed", created.StatusCode, readErr)
	}
	body, _ = json.Marshal(map[string]any{"offset": 0, "data": data, "sha256": hex.EncodeToString(hash[:])})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	reader, writer := io.Pipe()
	transport := paired.Transport.(*http.Transport).Clone()
	transport.ExpectContinueTimeout = time.Second
	upload := &nativeRevocableUpload{id: transfer.ID, cancel: cancel, ctx: ctx, reader: reader, transport: transport, release: make(chan struct{}), done: make(chan nativeUploadResult, 1)}
	req, err = http.NewRequestWithContext(ctx, "POST", origin+"/api/v1/transfers/"+transfer.ID+"/chunks", reader)
	if err != nil {
		upload.Close()
		t.Fatal(err)
	}
	req.ContentLength = int64(len(body))
	req.Header.Set("Origin", origin)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Expect", "100-continue")
	continued, prefix := make(chan struct{}), make(chan struct{})
	var signalled sync.Once
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{Got100Continue: func() { signalled.Do(func() { close(continued) }) }}))
	client := &http.Client{Transport: transport, Jar: paired.Jar}
	go func() { response, err := client.Do(req); upload.done <- nativeUploadResult{response, err} }()
	go func() {
		_, err := writer.Write(body[:len(body)/2])
		if err == nil {
			close(prefix)
			select {
			case <-upload.release:
				_, err = writer.Write(body[len(body)/2:])
			case <-ctx.Done():
				err = ctx.Err()
			}
		}
		writer.CloseWithError(err)
	}()
	for continued != nil || prefix != nil {
		select {
		case <-continued:
			continued = nil
		case <-prefix:
			prefix = nil
		case result := <-upload.done:
			if result.response != nil {
				result.response.Body.Close()
			}
			upload.Close()
			t.Fatal("upload settled before revocation", result.err)
		case <-ctx.Done():
			upload.Close()
			t.Fatal("upload did not establish a partial HTTPS body")
		}
	}
	return upload
}

func (u *nativeRevocableUpload) Close() {
	u.cancel()
	u.reader.CloseWithError(context.Canceled)
	u.transport.CloseIdleConnections()
}
func (u *nativeRevocableUpload) ExpectRevoked(t *testing.T) {
	t.Helper()
	u.once.Do(func() { close(u.release) })
	select {
	case result := <-u.done:
		if result.err != nil {
			u.Close()
			t.Fatal("revoked upload lost its explicit denial", result.err)
		}
		defer result.response.Body.Close()
		data, err := io.ReadAll(io.LimitReader(result.response.Body, 4097))
		var refusal struct{ Error struct{ Code string } }
		if err != nil || len(data) > 4096 || result.response.StatusCode != 401 || result.response.TLS == nil || len(result.response.TLS.VerifiedChains) == 0 || json.Unmarshal(data, &refusal) != nil || refusal.Error.Code != "AUTH_REQUIRED" || u.ctx.Err() != nil {
			u.Close()
			t.Fatal("revoked partial upload was not denied after body completion", result.response.StatusCode, err)
		}
	case <-u.ctx.Done():
		u.Close()
		t.Fatal("revoked partial upload did not settle")
	}
	u.Close()
	t.Logf("native HTTPS upload revocation transfer=%s denied after partial body; committed offset inspection follows", u.id)
}
