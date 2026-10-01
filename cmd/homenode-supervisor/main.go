package main

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/pranavreddyg17/home-node/internal/catalog"
	"github.com/pranavreddyg17/home-node/internal/state"
	"github.com/pranavreddyg17/home-node/internal/supervisor"
)

func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
func readProtected(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 {
		return nil, fmt.Errorf("unprotected configuration: %s", path)
	}
	if st, ok := info.Sys().(*syscall.Stat_t); !ok || st.Uid != 0 {
		return nil, fmt.Errorf("configuration must be root owned: %s", path)
	}
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) || opened.Mode().Perm()&0022 != 0 {
		return nil, fmt.Errorf("configuration changed while opening")
	}
	if st, ok := opened.Sys().(*syscall.Stat_t); !ok || st.Uid != 0 {
		return nil, fmt.Errorf("configuration ownership changed")
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("configuration exceeds limit")
	}
	return data, nil
}
func main() {
	config := flag.String("policy", "/etc/homenode/runtime-policy.json", "protected enforcement policy")
	publisher := flag.String("publisher-key", "/etc/homenode/catalog.pub", "pinned hex Ed25519 publisher key")
	floorPath := flag.String("catalog-floor", "/etc/homenode/catalog-floor", "protected independent minimum catalog version")
	manifestPath := flag.String("catalog", "/var/lib/homenode/catalog/catalog.json", "verified catalog envelope")
	root := flag.String("state-dir", "/var/lib/homenode/supervisor", "protected runtime journal")
	images := flag.String("images", "/var/lib/homenode/images", "immutable image directory")
	volumes := flag.String("volumes", "/var/lib/homenode/volumes", "protected raw volume directory")
	channels := flag.String("channels", "/run/homenode/guests", "transfer-only channels")
	socket := flag.String("socket", "/run/homenode/supervisor.sock", "local runtime socket")
	accessGID := flag.Int("access-gid", -1, "controller/transfer shared runtime group")
	transferGID := flag.Int("transfer-gid", -1, "transfer-only group")
	maintenanceUID := flag.Int("maintenance-uid", -1, "distinct backup service UID; disabled when omitted")
	maintenanceGID := flag.Int("maintenance-gid", -1, "distinct backup socket access group")
	maintenanceSocket := flag.String("maintenance-socket", "/run/homenode/maintenance.sock", "backup-only local maintenance socket")
	flag.Parse()
	if runtime.GOOS != "linux" || os.Geteuid() != 0 || *accessGID < 1 || *transferGID < 1 {
		fatal(fmt.Errorf("requires Linux, root service identity, and explicit service groups"))
	}
	policyBytes, err := readProtected(*config, 16384)
	if err != nil {
		fatal(err)
	}
	var policy supervisor.Policy
	decoder := json.NewDecoder(strings.NewReader(string(policyBytes)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&policy); err != nil {
		fatal(err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		fatal(fmt.Errorf("invalid trailing policy data"))
	}
	if err = policy.Validate(); err != nil {
		fatal(err)
	}
	if err = validateMaintenancePeer(*maintenanceUID, *maintenanceGID, *accessGID, *transferGID, policy); err != nil {
		fatal(err)
	}
	if *maintenanceUID != -1 && (!filepath.IsAbs(*maintenanceSocket) || filepath.Clean(*maintenanceSocket) != *maintenanceSocket || *maintenanceSocket == *socket) {
		fatal(errors.New("invalid maintenance socket"))
	}
	keyBytes, err := readProtected(*publisher, 256)
	if err != nil {
		fatal(err)
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(keyBytes)))
	if err != nil || len(key) != ed25519.PublicKeySize {
		fatal(fmt.Errorf("invalid publisher key"))
	}
	data, err := readProtected(*manifestPath, catalog.MaxManifestBytes)
	if err != nil {
		fatal(err)
	}
	floorBytes, err := readProtected(*floorPath, 32)
	if err != nil {
		fatal(err)
	}
	minimum, err := catalog.VersionFloor(floorBytes)
	if err != nil {
		fatal(err)
	}
	store, err := state.Open(*root)
	if err != nil {
		fatal(err)
	}
	defer store.Close()
	public := ed25519.PublicKey(key)
	manifest, err := acceptCatalog(context.Background(), store.DB, data, public, minimum, time.Now())
	if err != nil {
		fatal(err)
	}
	for _, directory := range []string{*images, *volumes, *channels} {
		if !filepath.IsAbs(directory) {
			fatal(fmt.Errorf("runtime directories must be absolute"))
		}
		if err = os.MkdirAll(directory, 0710); err != nil {
			fatal(err)
		}
	}
	qemuGroup, err := user.LookupGroup("libvirt-qemu")
	if err != nil {
		fatal(err)
	}
	qemuGID, err := strconv.Atoi(qemuGroup.Gid)
	if err != nil {
		fatal(err)
	}
	for _, directory := range []string{*images, *volumes} {
		if err = os.Chown(directory, 0, qemuGID); err != nil {
			fatal(err)
		}
		if err = os.Chmod(directory, 0710); err != nil {
			fatal(err)
		}
	}
	if err = os.Chown(*channels, 0, 0); err != nil {
		fatal(err)
	}
	if err = os.Chmod(*channels, 0755); err != nil {
		fatal(err)
	}
	manager := &supervisor.Manager{Store: store, Policy: policy, Manifest: manifest, Images: *images, Volumes: *volumes, Channels: *channels, Backend: supervisor.LinuxBackend{DataRoot: *volumes, TransferGID: *transferGID}}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err = manager.Initialize(ctx); err != nil {
		fatal(err)
	}
	if err = manager.Reconcile(ctx); err != nil {
		fatal(err)
	}
	listener, err := net.Listen("unix", *socket)
	if err != nil {
		fatal(err)
	}
	defer listener.Close()
	if err = os.Chown(*socket, 0, *accessGID); err != nil {
		fatal(err)
	}
	if err = os.Chmod(*socket, 0660); err != nil {
		fatal(err)
	}
	server := &http.Server{Handler: manager.Handler(), ConnContext: supervisor.PeerContext, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 95 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 8192}
	var maintenanceServer *http.Server
	maintenanceDone := make(chan error, 1)
	if *maintenanceUID != -1 {
		maintenanceListener, listenErr := net.Listen("unix", *maintenanceSocket)
		if listenErr != nil {
			fatal(listenErr)
		}
		defer maintenanceListener.Close()
		if err = os.Chown(*maintenanceSocket, 0, *maintenanceGID); err != nil {
			fatal(err)
		}
		if err = os.Chmod(*maintenanceSocket, 0660); err != nil {
			fatal(err)
		}
		maintenanceServer = &http.Server{Handler: manager.MaintenanceHandler(uint32(*maintenanceUID)), ConnContext: supervisor.PeerContext, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 95 * time.Second, IdleTimeout: 10 * time.Second, MaxHeaderBytes: 8192}
		go func() {
			serveErr := maintenanceServer.Serve(maintenanceListener)
			if errors.Is(serveErr, http.ErrServerClosed) {
				serveErr = nil
			}
			maintenanceDone <- serveErr
			stop()
		}()
	}
	shutdownDone := make(chan error, 1)
	go func() {
		<-ctx.Done()
		deadline, cancel := context.WithTimeout(context.Background(), 100*time.Second)
		maintenanceShutdown := make(chan error, 1)
		if maintenanceServer != nil {
			go func() { maintenanceShutdown <- maintenanceServer.Shutdown(deadline) }()
		}
		shutdownErr := server.Shutdown(deadline)
		if maintenanceServer != nil {
			maintenanceErr := <-maintenanceShutdown
			if maintenanceErr != nil {
				_ = maintenanceServer.Close()
			}
			shutdownErr = errors.Join(shutdownErr, maintenanceErr)
		}
		cancel()
		if shutdownErr != nil {
			_ = server.Close()
		}
		cleanup, cancelCleanup := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancelCleanup()
		shutdownDone <- errors.Join(shutdownErr, manager.Shutdown(cleanup))
	}()
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				deadline, cancel := context.WithTimeout(ctx, 60*time.Second)
				if err := manager.Audit(deadline); err != nil {
					fmt.Fprintln(os.Stderr, "Runtime audit requires attention:", err)
				}
				cancel()
			}
		}
	}()
	fmt.Println("HomeNode runtime supervisor ready")
	err = server.Serve(listener)
	stop()
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	if maintenanceServer != nil {
		err = errors.Join(err, <-maintenanceDone)
	}
	if err = errors.Join(err, <-shutdownDone); err != nil {
		fatal(err)
	}
}

func raiseCatalogFloor(configured int64, stored string) (int64, error) {
	if configured < 1 {
		return 0, catalog.ErrUntrusted
	}
	accepted, err := catalog.VersionFloor([]byte(stored))
	if err != nil {
		return 0, err
	}
	if accepted > configured {
		return accepted, nil
	}
	return configured, nil
}

func validateMaintenancePeer(uid, gid, accessGID, transferGID int, policy supervisor.Policy) error {
	if uid == -1 && gid == -1 {
		return nil
	}
	if uid < 1 || uint64(uid) > uint64(^uint32(0)) || gid < 1 || uint64(gid) > uint64(^uint32(0)) || uint32(uid) == policy.ControllerUID || uint32(uid) == policy.TransferUID || gid == accessGID || gid == transferGID {
		return errors.New("maintenance requires distinct non-root backup UID and socket group")
	}
	return nil
}
