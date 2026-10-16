//go:build linux

package gatewaytransport

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

// Opt-in disposable Linux root fixture; child processes have separate kernel
// identities. This qualifies socket mediation and fixture TLS, not Tailscale or
// installed gateway startup and confinement.
func TestRootDistinctControllerGatewayPeers(t *testing.T) {
	if os.Geteuid() != 0 || os.Getenv("HOMENODE_GATEWAY_PEER_INTEGRATION") != "1" {
		t.Skip("disposable Linux root peer fixture is opt-in")
	}
	root, err := os.MkdirTemp("/tmp", "homenode-native-peer-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Error(err)
		}
	})
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	input, err := os.Open(executable)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	binary := filepath.Join(root, "peer.test")
	output, err := os.OpenFile(binary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0755)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatal(copyErr, closeErr)
	}
	directory := filepath.Join(root, "controller")
	if err := os.Mkdir(directory, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(directory, 991, 993); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(directory, "control.sock")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	child := func(role string, uid, gid uint32) *exec.Cmd {
		cmd := exec.CommandContext(ctx, binary, "-test.run=^TestNativeGatewayPeerChild$", "-test.count=1")
		cmd.Env = []string{"PATH=/usr/bin:/bin", "HOMENODE_NATIVE_PEER_ROLE=" + role, "HOMENODE_NATIVE_PEER_SOCKET=" + socket}
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uid, Gid: gid, Groups: []uint32{993}}}
		cmd.WaitDelay = time.Second
		return cmd
	}
	server := child("controller", 991, 993)
	var serverLog bytes.Buffer
	server.Stdout = &serverLog
	server.Stderr = &serverLog
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { cancel(); _ = server.Wait() }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		info, err := os.Lstat(socket)
		if err == nil && info.Mode()&os.ModeSocket != 0 && info.Mode().Perm() == 0660 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("controller child did not establish protected socket")
		}
		time.Sleep(10 * time.Millisecond)
	}
	for _, entry := range []struct {
		role string
		uid  uint32
	}{{"gateway", 992}, {"gateway-tls", 992}, {"foreign", 994}} {
		cmd := child(entry.role, entry.uid, 993)
		if data, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s kernel peer fixture: %v (%s)", entry.role, err, data)
		}
		if entry.role == "gateway-tls" {
			t.Log("distinct gateway/controller kernel peers passed actual certificate-verified TLS 1.3 mediation and TLS 1.2 refusal; tailnet address remains synthetic")
		}
	}
	// Filesystem labels alone cannot authenticate the serving process. Give the
	// socket an expected foreign owner while the actual listener remains UID 991.
	if err := os.Chown(directory, 0, 993); err != nil {
		t.Fatal(err)
	}
	if err := os.Chown(socket, 990, 993); err != nil {
		t.Fatal(err)
	}
	if data, err := child("foreign-controller", 992, 993).CombinedOutput(); err != nil {
		t.Fatalf("foreign serving kernel peer accepted: %v (%s)", err, data)
	}
}

// Executed only in children selected by the parent fixture. The controller is
// deliberately minimal: the shipped peer context and transport handlers remain
// identical to production; application authentication is outside this fixture.
func TestNativeGatewayPeerChild(t *testing.T) {
	role := os.Getenv("HOMENODE_NATIVE_PEER_ROLE")
	if role == "" {
		t.Skip("native child only")
	}
	socket := os.Getenv("HOMENODE_NATIVE_PEER_SOCKET")
	if role == "controller" {
		if os.Geteuid() != 991 {
			t.Fatal("controller kernel identity")
		}
		listener, err := net.Listen("unix", socket)
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		if err := os.Chmod(socket, 0660); err != nil {
			t.Fatal(err)
		}
		handler, err := ControllerHandler(992, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !TrustedHTTPS(r) || r.RemoteAddr != "100.64.0.2:0" || r.Header.Get("Forwarded") != "" || r.Header.Get(clientAddressHeader) != "" || r.Header.Get(clientTLSHeader) != "" {
				http.Error(w, "mediation refused", 500)
				return
			}
			_, _ = w.Write([]byte("qualified peer"))
		}))
		if err != nil {
			t.Fatal(err)
		}
		server := &http.Server{Handler: handler, ConnContext: supervisor.PeerContext, ReadHeaderTimeout: time.Second}
		if err := server.Serve(listener); err != nil {
			t.Fatal(err)
		}
		return
	}
	uid := 992
	want := 200
	controllerUID := uint32(991)
	if role == "foreign" {
		uid = 994
		want = 403
	} else if role == "foreign-controller" {
		controllerUID = 990
		want = 503
	} else if role != "gateway" && role != "gateway-tls" {
		t.Fatal("unknown child role")
	}
	if os.Geteuid() != uid {
		t.Fatal("client kernel identity")
	}
	origin := "https://home.example.test:8787"
	if role == "gateway-tls" {
		origin = "https://example.com:8787"
	}
	proxy, closeProxy, err := NewProxy(Config{Origin: origin, Socket: socket, ControllerUID: controllerUID, AccessGID: 993})
	if err != nil {
		t.Fatal(err)
	}
	defer closeProxy()
	if role == "gateway-tls" {
		nativeGatewayTLS(t, proxy, origin)
		return
	}
	request := httptest.NewRequest("GET", "/api/v1/healthz", nil)
	request.Host = "home.example.test:8787"
	request.RemoteAddr = "100.64.0.2:12345"
	request.TLS = &tls.ConnectionState{Version: tls.VersionTLS13}
	request.Header.Set("Forwarded", "for=100.64.0.99;proto=http")
	request.Header.Set(clientAddressHeader, "100.64.0.99")
	request.Header.Set(clientTLSHeader, "unsafe")
	response := httptest.NewRecorder()
	proxy.ServeHTTP(response, request)
	if response.Code != want || (want == 200 && response.Body.String() != "qualified peer") {
		t.Fatal("peer mediation status", strconv.Itoa(response.Code))
	}
}

// Real loopback TLS and kernel Unix peers, with a synthetic tailnet address.
// Certificate file ownership, installed services and overlay ACLs are separate.
func nativeGatewayTLS(t *testing.T, proxy http.Handler, origin string) {
	t.Helper()
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS == nil || r.TLS.Version != tls.VersionTLS13 {
			t.Error("missing actual TLS 1.3 connection state")
		}
		r.RemoteAddr = "100.64.0.2:12345"
		proxy.ServeHTTP(w, r)
	}))
	server.TLS = &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13}
	server.StartTLS()
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	transport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13}, DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	request, err := http.NewRequest("GET", origin+"/api/v1/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Forwarded", "for=100.64.0.99;proto=http")
	request.Header.Set(clientAddressHeader, "100.64.0.99")
	request.Header.Set(clientTLSHeader, "unsafe")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal("verified TLS gateway request", err)
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 4096))
	response.Body.Close()
	if readErr != nil || response.StatusCode != 200 || string(body) != "qualified peer" || response.TLS == nil || response.TLS.Version != tls.VersionTLS13 || len(response.TLS.VerifiedChains) == 0 {
		t.Fatal("TLS/peer mediation failed", response.StatusCode, string(body), readErr)
	}
	wrongHost, err := http.NewRequest("GET", origin+"/api/v1/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}
	wrongHost.Host = "foreign.example:8787"
	refused, err := client.Do(wrongHost)
	if err != nil {
		t.Fatal(err)
	}
	refused.Body.Close()
	if refused.StatusCode != http.StatusForbidden {
		t.Fatal("foreign HTTP origin crossed gateway", refused.StatusCode)
	}
	for _, scenario := range []string{"untrusted-root", "wrong-server-name"} {
		untrusted := transport.Clone()
		if scenario == "untrusted-root" {
			untrusted.TLSClientConfig.RootCAs = x509.NewCertPool()
		} else {
			untrusted.TLSClientConfig.ServerName = "foreign.example"
		}
		response, err := (&http.Client{Transport: untrusted, Timeout: 3 * time.Second}).Get(origin + "/api/v1/healthz")
		untrusted.CloseIdleConnections()
		if err == nil {
			response.Body.Close()
			t.Fatal("unqualified certificate accepted", scenario)
		}
	}
	legacy := transport.Clone()
	legacy.TLSClientConfig.MinVersion, legacy.TLSClientConfig.MaxVersion = tls.VersionTLS12, tls.VersionTLS12
	defer legacy.CloseIdleConnections()
	if response, err := (&http.Client{Transport: legacy, Timeout: 3 * time.Second}).Get(origin + "/api/v1/healthz"); err == nil {
		response.Body.Close()
		t.Fatal("TLS 1.2 handshake was accepted")
	}
}
