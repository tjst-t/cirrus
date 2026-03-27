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
	OVNNB             string `yaml:"ovn_nb"`
	StorageEndpoint   string `yaml:"storage_endpoint"`
	AWXEndpoint       string `yaml:"awx_endpoint"`
	NetBoxEndpoint    string `yaml:"netbox_endpoint"`
	AuthTokens        string `yaml:"auth_tokens"`        // comma-separated token=external_id pairs
	RegistrationToken string `yaml:"registration_token"` // shared secret for worker self-registration
	LogLevel          string `yaml:"log_level"`          // debug, info, warn, error
}

// WorkerConfig holds configuration for the worker process.
type WorkerConfig struct {
	Controller         string `yaml:"controller"`
	HostID             string `yaml:"host_id"`
	LibvirtURI         string `yaml:"libvirt_uri"`
	RegistrationToken  string `yaml:"registration_token"`  // shared secret for self-registration
	HeartbeatInterval  int    `yaml:"heartbeat_interval"`  // heartbeat interval in seconds
	LogLevel           string `yaml:"log_level"`           // debug, info, warn, error
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
