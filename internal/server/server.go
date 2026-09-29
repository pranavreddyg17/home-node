package server

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/pranavreddyg17/home-node/internal/hostcheck"
)

func New(report func() hostcheck.Report) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/healthz", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, map[string]string{"status": "alive"})
	})
	mux.HandleFunc("GET /api/v1/host/report", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, report())
	})
	mux.HandleFunc("GET /api/v1/ready", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusServiceUnavailable, map[string]string{"status": "execution_disabled", "reason": "VM isolation has not been implemented and verified"})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if !isLoopbackHost(r.Host) || r.URL.IsAbs() {
			http.Error(w, "invalid host", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func isLoopbackHost(host string) bool {
	name := host
	if strings.Contains(host, ":") {
		var err error
		var port string
		name, port, err = net.SplitHostPort(host)
		if err != nil {
			return false
		}
		value, err := strconv.ParseUint(port, 10, 16)
		if err != nil || value == 0 {
			return false
		}
	}
	return name == "localhost" || name == "127.0.0.1"
}

func jsonResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
