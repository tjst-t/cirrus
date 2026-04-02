package storage

import "context"

// Driver abstracts communication with a storage backend.
// Implementations exist per backend type (sim, iscsi, rbd, nfs, ...).
// Driver runs in the Controller and calls the backend's management API.
// Host-side OS-level attachment (iscsiadm, rbd map) is handled by
// the Worker's blockdev.Manager using the ExportInfo returned here.
type Driver interface {
	// CreateVolume creates a new volume on the backend.
	CreateVolume(ctx context.Context, spec DriverVolumeSpec) (*DriverVolume, error)
	// DeleteVolume removes a volume from the backend.
	DeleteVolume(ctx context.Context, volumeID string) error
	// ResizeVolume grows a volume to newSizeGB. Shrink is not allowed.
	ResizeVolume(ctx context.Context, volumeID string, newSizeGB int64) error
	// ExportVolume configures backend-side access for the given host
	// (e.g. iSCSI ACL entry, RBD keyring grant) and returns connection info.
	// host.Properties carries protocol-specific attributes (iscsi_iqn, etc.)
	// sourced from hosts.storage_properties in the DB.
	ExportVolume(ctx context.Context, volumeID string, host HostInfo) (*ExportInfo, error)
	// UnexportVolume revokes backend-side access for the given host.
	UnexportVolume(ctx context.Context, volumeID string, host HostInfo) error
	// Capabilities returns the feature set supported by this driver/backend.
	Capabilities() DriverCapabilities
}

// HostInfo carries host attributes needed by the Driver to configure
// backend-side access. Assembled by the Storage Service from the DB.
type HostInfo struct {
	ID         string
	DataIPs    []string
	Properties map[string]string // from hosts.storage_properties
}

// ExportInfo is the connection information returned by ExportVolume.
// Passed to the Worker via gRPC (DiskSpec) so blockdev.Manager can
// perform the OS-level attachment.
type ExportInfo struct {
	Protocol string            // "rbd", "iscsi", "nfs", ...
	Params   map[string]string // protocol-specific parameters
}

// DriverVolumeSpec is the input for CreateVolume.
type DriverVolumeSpec struct {
	VolumeID        string
	SizeGB          int64
	ThinProvisioned bool
}

// DriverVolume is the result of CreateVolume.
type DriverVolume struct {
	VolumeID string
	SizeGB   int64
}

// DriverCapabilities declares which optional features the backend supports.
type DriverCapabilities struct {
	QoS                  bool
	Encryption           bool
	Replication          bool
	DifferentialTransfer bool
	StorageLiveMigration bool
	Snapshot             bool
	Clone                bool
}
