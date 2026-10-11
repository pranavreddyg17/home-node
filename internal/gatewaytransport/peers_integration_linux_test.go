//go:build linux

package gatewaytransport

import (
	"bytes"
	"context"
	"crypto/tls"
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
// identities. This qualifies socket mediation, not Tailscale or HTTPS startup.
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
	}{{"gateway", 992}, {"foreign", 994}} {
		cmd := child(entry.role, entry.uid, 993)
		if data, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s kernel peer fixture: %v (%s)", entry.role, err, data)
		}
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
	if role == "foreign" {
		uid = 994
		want = 403
	} else if role != "gateway" {
		t.Fatal("unknown child role")
	}
	if os.Geteuid() != uid {
		t.Fatal("client kernel identity")
	}
	proxy, closeProxy, err := NewProxy(Config{Origin: "https://home.example.test:8787", Socket: socket, ControllerUID: 991, AccessGID: 993})
	if err != nil {
		t.Fatal(err)
	}
	defer closeProxy()
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
