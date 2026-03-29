// Package webui provides the cirrus-sim operations dashboard.
package webui

import (
	"context"
	"embed"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

//go:embed static
var staticFiles embed.FS

// Endpoints maps simulator names to their base URLs for proxying.
type Endpoints map[string]string

// DefaultEndpoints returns the standard simulator endpoint mapping.
func DefaultEndpoints() Endpoints {
	return Endpoints{
		"common":      "http://localhost:8000",
		"libvirt-sim": "http://localhost:8100",
		"ovn-sim":     "http://localhost:8200",
		"awx-sim":     "http://localhost:8300",
		"netbox-sim":  "http://localhost:8400",
		"storage-sim": "http://localhost:8500",
	}
}

// Version is the application version, set by the caller (typically from ldflags).
var Version = "dev"

// Server is the dashboard web UI server.
type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
}

// New creates a new dashboard Server that proxies to the given endpoints.
func New(port string, endpoints Endpoints, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}

	mux := http.NewServeMux()

	// Proxy endpoints (register before the catch-all)
	for name, baseURL := range endpoints {
		proxyPath := fmt.Sprintf("GET /proxy/%s/", name)
		mux.Handle(proxyPath, newProxy(fmt.Sprintf("/proxy/%s/", name), baseURL))
	}

	// Serve embedded static files (catch-all for GET /)
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data, err := staticFiles.ReadFile("static/index.html")
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, writeErr := w.Write(data); writeErr != nil {
			logger.Warn("failed to write response", "error", writeErr)
		}
	})

	// Version endpoint
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"version":%q}`, Version)
	})

	// Aggregated status
	mux.HandleFunc("GET /api/status", statusHandler(endpoints))

	return &Server{
		httpServer: &http.Server{
			Addr:              fmt.Sprintf(":%s", port),
			Handler:           mux,
			ReadHeaderTimeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// Start starts the server in a goroutine.
func (s *Server) Start() {
	go func() {
		s.logger.Info("dashboard starting", "addr", s.httpServer.Addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("dashboard server failed", "error", err)
		}
	}()
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

type proxy struct {
	prefix  string
	baseURL string
	client  *http.Client
}

func newProxy(prefix, baseURL string) *proxy {
	return &proxy{prefix: prefix, baseURL: baseURL, client: &http.Client{Timeout: 5 * time.Second}}
}

func (p *proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	targetPath := r.URL.Path[len(p.prefix)-1:]
	targetURL := p.baseURL + targetPath
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	proxyReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("proxy error: %v", err), http.StatusBadGateway)
		return
	}
	proxyReq.Header = r.Header.Clone()

	resp, err := p.client.Do(proxyReq)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		fmt.Fprintf(w, `{"error":"service unavailable"}`)
		return
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			slog.Warn("close proxy response", "error", closeErr)
		}
	}()

	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if _, copyErr := io.Copy(w, resp.Body); copyErr != nil {
		slog.Warn("copy proxy response", "error", copyErr)
	}
}

func statusHandler(endpoints Endpoints) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client := &http.Client{Timeout: 2 * time.Second}

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"simulators":[`)
		first := true
		for name, baseURL := range endpoints {
			if !first {
				fmt.Fprint(w, ",")
			}
			first = false

			status := "down"
			statsURL := baseURL + "/sim/stats"
			if name == "common" {
				statsURL = baseURL + "/api/v1/events?limit=1"
			}

			req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, statsURL, nil)
			if err == nil {
				resp, doErr := client.Do(req)
				if doErr == nil {
					if resp.StatusCode == http.StatusOK {
						status = "up"
					}
					resp.Body.Close()
				}
			}
			fmt.Fprintf(w, `{"name":"%s","url":"%s","status":"%s"}`, name, baseURL, status)
		}
		fmt.Fprint(w, `]}`)
	}
}
