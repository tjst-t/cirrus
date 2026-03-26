package config

// ControllerConfig holds configuration for the controller process.
type ControllerConfig struct {
	APIPort         int    `yaml:"api_port"`
	GRPCPort        int    `yaml:"grpc_port"`
	DBDSN           string `yaml:"db_dsn"`
	OVNNB           string `yaml:"ovn_nb"`
	StorageEndpoint string `yaml:"storage_endpoint"`
	AWXEndpoint     string `yaml:"awx_endpoint"`
	NetBoxEndpoint  string `yaml:"netbox_endpoint"`
}

// WorkerConfig holds configuration for the worker process.
type WorkerConfig struct {
	Controller string `yaml:"controller"`
	HostID     string `yaml:"host_id"`
	LibvirtURI string `yaml:"libvirt_uri"`
}
