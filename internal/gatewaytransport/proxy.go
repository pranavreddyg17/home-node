// Package gatewaytransport defines the fixed gateway/controller HTTP boundary.
// It grants no runtime authority and exposes no caller-selected upstream.
package gatewaytransport

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

const clientAddressHeader = "X-Homenode-Client-Address"
const clientTLSHeader = "X-Homenode-Client-Tls"

var tailnet = netip.MustParsePrefix("100.64.0.0/10")
var ErrTransport = errors.New("gateway controller transport denied")

type Config struct {
	Origin        string
	Socket        string
	ControllerUID uint32
	AccessGID     uint32
}

func NewProxy(config Config) (http.Handler, func(), error) {
	origin, err := url.Parse(config.Origin)
	if err != nil || origin.Scheme != "https" || origin.Host == "" || origin.User != nil || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" || origin.ForceQuery || origin.Opaque != "" || config.ControllerUID == 0 || config.AccessGID == 0 || !filepath.IsAbs(config.Socket) || filepath.Clean(config.Socket) != config.Socket {
		return nil, nil, ErrTransport
	}
	transport := &http.Transport{Proxy: nil, MaxConnsPerHost: 32, MaxIdleConnsPerHost: 8, IdleConnTimeout: 30 * time.Second, ResponseHeaderTimeout: 95 * time.Second,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) { return dialController(ctx, config) },
	}
	proxy := &httputil.ReverseProxy{Transport: transport, FlushInterval: -1,
		ModifyResponse: func(response *http.Response) error {
			if response.StatusCode == http.StatusSwitchingProtocols {
				return ErrTransport
			}
			return nil
		},
		Rewrite: func(request *httputil.ProxyRequest) {
			request.Out.URL.Scheme, request.Out.URL.Host = "http", "controller.invalid"
			request.Out.Host = origin.Host
			// No public forwarding assertion survives into the private boundary.
			for name := range request.Out.Header {
				lower := strings.ToLower(name)
				if strings.HasPrefix(lower, "x-homenode-") || strings.HasPrefix(lower, "x-forwarded-") || lower == "forwarded" {
					delete(request.Out.Header, name)
				}
			}
			address, _ := netip.ParseAddrPort(request.In.RemoteAddr)
			request.Out.Header.Set(clientAddressHeader, address.Addr().Unmap().String())
			request.Out.Header.Set(clientTLSHeader, "1.3")
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"error":{"code":"GATEWAY_UNAVAILABLE","message":"The controller is unavailable. Try again shortly."}}`))
		},
	}
	slots := make(chan struct{}, 64)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w = &deadlineWriter{ResponseWriter: w}
		address, err := netip.ParseAddrPort(r.RemoteAddr)
		if err != nil || !tailnet.Contains(address.Addr().Unmap()) || r.TLS == nil || r.TLS.Version != tls.VersionTLS13 || r.Host != origin.Host || r.URL.IsAbs() || r.URL.Host != "" || r.Method == "CONNECT" || r.Header.Get("Upgrade") != "" || len(r.Trailer) != 0 {
			http.Error(w, "gateway request denied", http.StatusForbidden)
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			http.Error(w, "gateway busy", http.StatusServiceUnavailable)
			return
		}
		proxy.ServeHTTP(w, r)
	})
	return handler, transport.CloseIdleConnections, nil
}

// CheckController authenticates the installed peer without sending an HTTP
// operation. A gateway does not publish its TLS listener before this succeeds.
func CheckController(ctx context.Context, config Config) error {
	connection, err := dialController(ctx, config)
	if err != nil {
		return err
	}
	return connection.Close()
}

func dialController(ctx context.Context, config Config) (net.Conn, error) {
	if config.ControllerUID == 0 || config.AccessGID == 0 || !filepath.IsAbs(config.Socket) || filepath.Clean(config.Socket) != config.Socket {
		return nil, ErrTransport
	}
	before, err := os.Lstat(config.Socket)
	if err != nil || before.Mode()&os.ModeSocket == 0 || before.Mode().Perm() != 0660 || before.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return nil, ErrTransport
	}
	metadata, ok := before.Sys().(*syscall.Stat_t)
	if !ok || metadata.Uid != config.ControllerUID || metadata.Gid != config.AccessGID || metadata.Nlink != 1 {
		return nil, ErrTransport
	}
	parent, err := os.Lstat(filepath.Dir(config.Socket))
	if err != nil || !parent.IsDir() || parent.Mode().Perm()&0022 != 0 {
		return nil, ErrTransport
	}
	owner, ok := parent.Sys().(*syscall.Stat_t)
	if !ok || (owner.Uid != 0 && owner.Uid != config.ControllerUID) {
		return nil, ErrTransport
	}
	conn, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", config.Socket)
	if err != nil {
		return nil, ErrTransport
	}
	peer, err := supervisor.PeerUID(conn.(*net.UnixConn))
	after, statErr := os.Lstat(config.Socket)
	if err != nil || statErr != nil || peer != config.ControllerUID || !os.SameFile(before, after) || before.Mode() != after.Mode() {
		conn.Close()
		return nil, ErrTransport
	}
	updated, ok := after.Sys().(*syscall.Stat_t)
	if !ok || updated.Uid != metadata.Uid || updated.Gid != metadata.Gid || updated.Nlink != metadata.Nlink {
		conn.Close()
		return nil, ErrTransport
	}
	return conn, nil
}

type deadlineWriter struct{ http.ResponseWriter }

func (w *deadlineWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *deadlineWriter) WriteHeader(status int) {
	_ = http.NewResponseController(w.ResponseWriter).SetWriteDeadline(time.Now().Add(15 * time.Second))
	w.ResponseWriter.WriteHeader(status)
}
func (w *deadlineWriter) Write(data []byte) (int, error) {
	_ = http.NewResponseController(w.ResponseWriter).SetWriteDeadline(time.Now().Add(15 * time.Second))
	return w.ResponseWriter.Write(data)
}
