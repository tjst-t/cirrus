package config

import (
	"log/slog"
	"strings"
)

// ControllerConfig holds configuration for the controller process.
type ControllerConfig struct {
	APIPort           int    `yaml:"api_port"`
	GRPCPort          int    `yaml:"grpc_port"`
	DBDSN             string `yaml:"db_dsn"`
	StorageEndpoint   string `yaml:"storage_endpoint"`
	FencingSimURL     string `yaml:"fencing_sim_url"`     // HTTP URL for SimFencingAgent (e.g. http://localhost:8100)
	AWXEndpoint       string `yaml:"awx_endpoint"`
	NetBoxEndpoint    string `yaml:"netbox_endpoint"`
	AuthTokens        string `yaml:"auth_tokens"`        // comma-separated token=external_id pairs
	RegistrationToken string `yaml:"registration_token"` // shared secret for worker self-registration
	LogLevel          string `yaml:"log_level"`          // debug, info, warn, error
	Debug             bool   `yaml:"debug"`              // include internal error details in API 500 responses

	// SecretsKey is a base64-encoded 32-byte key used for AES-GCM encryption of
	// secrets such as WireGuard private keys stored in the database.
	SecretsKey string `yaml:"secrets_key"`

	// Reconciler settings
	ReconcileInterval          int  `yaml:"reconcile_interval"`           // seconds; default 300
	ReconcileNetworkInterval   int  `yaml:"reconcile_network_interval"`   // seconds; 0 = use ReconcileInterval
	ReconcileStorageInterval   int  `yaml:"reconcile_storage_interval"`   // seconds; 0 = use ReconcileInterval
	ReconcileEnabled           bool `yaml:"reconcile_enabled"`            // default true
	AutoHealEnabled            bool `yaml:"auto_heal_enabled"`            // default true
	UnexpectedPresentThreshold int  `yaml:"unexpected_present_threshold"` // default 3
	DriftEventRetentionDays    int  `yaml:"drift_event_retention_days"`   // default 90
}

// WorkerConfig holds configuration for the worker process.
type WorkerConfig struct {
	Controller            string `yaml:"controller"`
	HostID                string `yaml:"host_id"`
	GRPCPort              int    `yaml:"grpc_port"`             // port for the worker-side WorkerService gRPC server
	WorkerGRPCAddr        string `yaml:"worker_grpc_addr"`      // address advertised to controller for calling WorkerService (host:port)
	LibvirtURI            string `yaml:"libvirt_uri"`
	LibvirtSimMgmtAddr    string `yaml:"libvirt_sim_mgmt_addr"` // HTTP management API base URL for libvirtd-sim (e.g. http://localhost:8100)
	RegistrationToken     string `yaml:"registration_token"`    // shared secret for self-registration
	HeartbeatInterval     int    `yaml:"heartbeat_interval"`    // heartbeat interval in seconds
	LogLevel              string `yaml:"log_level"`             // debug, info, warn, error
	StorageDomains        string `yaml:"storage_domains"`       // comma-separated storage domains to join
	Location              string `yaml:"location"`              // location in the topology tree (name or ID)
	FabricIP              string `yaml:"fabric_ip"`             // IP for Geneve tunnel endpoints (overlay fabric)
	GatewayUplinkPort     string `yaml:"gw_uplink_port"`        // Physical port for VLAN trunk (GW-role hosts only)
}

// ParseAuthTokens parses the auth-tokens flag value into a map of token→externalID.
func ParseAuthTokens(raw string) map[string]string {
	if raw == "" {
		return nil
	}
	tokens := make(map[string]string)
	for _, pair := range strings.Split(raw, ",") {
		k, v, ok := strings.Cut(pair, "=")
		if ok && k != "" && v != "" {
			tokens[k] = v
		}
	}
	return tokens
}

// ParseLogLevel converts a string log level to slog.Level. Defaults to Info.
func ParseLogLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
