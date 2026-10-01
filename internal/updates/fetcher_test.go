package updates

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/theupdateframework/go-tuf/v2/metadata"
)

func TestMetadataRepositoryPolicy(t *testing.T) {
	for _, address := range []string{"http://example.com/metadata", "https://user@example.com/metadata", "https://example.com/metadata?", "https://example.com/metadata?q=x", "https://example.com/metadata#x", "https://example.com/a/../metadata", "https://example.com/%6detadata"} {
		if _, err := newMetadataFetcher(context.Background(), address); !errors.Is(err, errDownloadPolicy) {
			t.Fatal(address, err)
		}
	}
	f, err := newMetadataFetcher(context.Background(), "https://example.com/metadata")
	if err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{"https://other.example.com/metadata/root.json", "https://example.com/metadata-other/root.json", "https://example.com/metadata/../root.json", "https://example.com/metadata/%2e%2e/root.json", "https://example.com/metadata/root.json?x=1"} {
		if _, err = f.DownloadFile(address, 100, 0); !errors.Is(err, errDownloadPolicy) {
			t.Fatal(address, err)
		}
	}
	for _, maximum := range []int64{0, -1, maxMetadataDownload + 1} {
		if _, err = f.DownloadFile("https://example.com/metadata/root.json", maximum, 0); !errors.Is(err, errDownloadPolicy) {
			t.Fatal(maximum, err)
		}
	}
}

func TestMetadataDownloadBoundsRedirectAndStatus(t *testing.T) {
	var redirected atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/metadata/exact":
			_, _ = w.Write([]byte("1234"))
		case "/metadata/overflow":
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			_, _ = w.Write([]byte("12345"))
		case "/metadata/redirect":
			http.Redirect(w, r, "/metadata/destination", http.StatusFound)
		case "/metadata/destination":
			redirected.Add(1)
		case "/metadata/compressed":
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write([]byte("1234"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	f, err := newMetadataFetcher(context.Background(), server.URL+"/metadata")
	if err != nil {
		t.Fatal(err)
	}
	f.client.Transport = server.Client().Transport
	data, err := f.DownloadFile(server.URL+"/metadata/exact", 4, 0)
	if err != nil || string(data) != "1234" {
		t.Fatal(string(data), err)
	}
	for _, resource := range []string{"overflow", "redirect", "compressed"} {
		if _, err = f.DownloadFile(server.URL+"/metadata/"+resource, 4, 0); err == nil {
			t.Fatal("unsafe response accepted", resource)
		}
	}
	if redirected.Load() != 0 {
		t.Fatal("redirect followed")
	}
	_, err = f.DownloadFile(server.URL+"/metadata/2.root.json", 4, 0)
	var missing *metadata.ErrDownloadHTTP
	if !errors.As(err, &missing) || missing.StatusCode != 404 {
		t.Fatal("TUF rotation stop signal lost", err)
	}
}

func TestMetadataDownloadCancellationDuringBody(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f, err := newMetadataFetcher(ctx, server.URL+"/metadata")
	if err != nil {
		t.Fatal(err)
	}
	f.client.Transport = server.Client().Transport
	done := make(chan error, 1)
	go func() { _, err := f.DownloadFile(server.URL+"/metadata/root.json", 1024, 0); done <- err }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("canceled body stayed active")
	}
	if _, err = f.DownloadFile(server.URL+"/metadata/root.json", 1024, 0); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled operation reused", err)
	}
}
