package agent

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
)

// MetadataResponse is the JSON structure returned by the metadata service.
type MetadataResponse struct {
	VMID        string              `json:"vm_id"`
	VMName      string              `json:"vm_name"`
	Hostname    string              `json:"hostname"`
	NetworkID   string              `json:"network_id"`
	NetworkName string              `json:"network_name"`
	GroupID     string              `json:"group_id"`
	GroupName   string              `json:"group_name"`
	Interfaces  []MetadataInterface `json:"interfaces"`
}

// MetadataInterface describes a network interface in metadata.
type MetadataInterface struct {
	MAC       string `json:"mac"`
	IP        string `json:"ip"`
	Gateway   string `json:"gateway"`
	Netmask   string `json:"netmask"`
	DNS       string `json:"dns"`
	NetworkID string `json:"network_id"`
}

// MetadataServer responds to VM metadata requests on 169.254.169.254.
type MetadataServer struct {
	cache  *StateCache
	logger *slog.Logger
	server *http.Server
}

// NewMetadataServer creates a new metadata HTTP server.
func NewMetadataServer(cache *StateCache, logger *slog.Logger) *MetadataServer {
	s := &MetadataServer{
		cache:  cache,
		logger: logger,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/meta-data/", s.handleMetadata)
	s.server = &http.Server{Handler: mux}
	return s
}

// ListenAndServe starts the metadata server on the given address.
func (s *MetadataServer) ListenAndServe(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("metadata: listen: %w", err)
	}
	return s.server.Serve(ln)
}

// Close stops the metadata server.
func (s *MetadataServer) Close() {
	if s.server != nil {
		s.server.Close()
	}
}

// ServeHTTP allows MetadataServer to be used as http.Handler for testing.
func (s *MetadataServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.server.Handler.ServeHTTP(w, r)
}

func (s *MetadataServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintln(w, "meta-data/")
}

func (s *MetadataServer) handleMetadata(w http.ResponseWriter, r *http.Request) {
	clientIP := extractHTTPClientIP(r)
	port := s.cache.GetPortByIP(clientIP)
	if port == nil {
		s.logger.Debug("metadata: unknown client IP", "ip", clientIP)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	resp := BuildMetadataResponse(port)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// BuildMetadataResponse constructs the metadata JSON from a PortState.
func BuildMetadataResponse(port interface{ GetIpAddress() string }) *MetadataResponse {
	// Type assert to get the full PortState
	type portStatelike interface {
		GetPortId() string
		GetVmId() string
		GetVmName() string
		GetNetworkId() string
		GetNetworkName() string
		GetGroupId() string
		GetGroupName() string
		GetMacAddress() string
		GetIpAddress() string
		GetGatewayIp() string
	}
	p, ok := port.(portStatelike)
	if !ok {
		return &MetadataResponse{}
	}

	return &MetadataResponse{
		VMID:        p.GetVmId(),
		VMName:      p.GetVmName(),
		Hostname:    p.GetVmName(),
		NetworkID:   p.GetNetworkId(),
		NetworkName: p.GetNetworkName(),
		GroupID:     p.GetGroupId(),
		GroupName:   p.GetGroupName(),
		Interfaces: []MetadataInterface{
			{
				MAC:       p.GetMacAddress(),
				IP:        p.GetIpAddress(),
				Gateway:   p.GetGatewayIp(),
				Netmask:   "255.255.255.252", // /30
				DNS:       p.GetGatewayIp(),
				NetworkID: p.GetNetworkId(),
			},
		},
	}
}

// extractHTTPClientIP gets the client IP from an HTTP request.
func extractHTTPClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
