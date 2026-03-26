package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Role       string           `yaml:"role"` // "controller" or "worker"
	Listen     string           `yaml:"listen"`
	Advertise  string           `yaml:"advertise"`
	Controller ControllerConfig `yaml:"controller"`
	Worker     WorkerConfig     `yaml:"worker"`
	Compute    ComputeConfig    `yaml:"compute"`
	Network    NetworkConfig    `yaml:"network"`
	Storage    StorageConfig    `yaml:"storage"`
	Image      ImageConfig      `yaml:"image"`
}

type ControllerConfig struct {
	DB         string `yaml:"db"`
	GRPCListen string `yaml:"grpc_listen"`
}

type WorkerConfig struct {
	ControllerAddr string `yaml:"controller_addr"`
	Name           string `yaml:"name"`
	VCPUs          int    `yaml:"vcpus"`
	RamMB          int    `yaml:"ram_mb"`
	DiskGB         int    `yaml:"disk_gb"`
}

type ComputeConfig struct {
	Driver string `yaml:"driver"`
	URI    string `yaml:"uri"`
}

type NetworkConfig struct {
	Driver  string `yaml:"driver"`
	Bridge  string `yaml:"bridge"`
	LocalIP string `yaml:"local_ip"`
}

type StorageConfig struct {
	Driver   string `yaml:"driver"`
	DiskDir  string `yaml:"disk_dir"`
	ImageDir string `yaml:"image_dir"`
}

type ImageConfig struct {
	Driver string `yaml:"driver"`
	Dir    string `yaml:"dir"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	cfg.setDefaults()
	return &cfg, nil
}

func (c *Config) setDefaults() {
	if c.Listen == "" {
		if c.Role == "controller" {
			c.Listen = "0.0.0.0:8080"
		} else {
			c.Listen = "0.0.0.0:9090"
		}
	}
	if c.Controller.GRPCListen == "" {
		c.Controller.GRPCListen = "0.0.0.0:9090"
	}
	if c.Compute.Driver == "" {
		c.Compute.Driver = "libvirt"
	}
	if c.Compute.URI == "" {
		c.Compute.URI = "qemu:///system"
	}
	if c.Network.Driver == "" {
		c.Network.Driver = "ovs"
	}
	if c.Network.Bridge == "" {
		c.Network.Bridge = "br-int"
	}
	if c.Storage.Driver == "" {
		c.Storage.Driver = "local"
	}
	if c.Storage.DiskDir == "" {
		c.Storage.DiskDir = "/var/lib/cirrus/disks"
	}
	if c.Storage.ImageDir == "" {
		c.Storage.ImageDir = "/var/lib/cirrus/images"
	}
	if c.Image.Dir == "" {
		c.Image.Dir = "/var/lib/cirrus/images"
	}
}
