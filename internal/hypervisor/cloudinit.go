package hypervisor

import (
	"fmt"
	"os"
	"path/filepath"

	diskfs "github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
	"github.com/diskfs/go-diskfs/filesystem/iso9660"
)

// BuildCloudInitISO creates an ISO 9660 image containing cloud-init seed files
// and writes it to dir/<hostname>-cidata.iso. It returns the ISO file path.
func BuildCloudInitISO(spec CloudInitSpec, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("cloudinit: mkdir: %w", err)
	}

	isoPath := filepath.Join(dir, spec.Hostname+"-cidata.iso")

	// Determine ISO size: content is small (a few KB), 4 MB is plenty.
	const isoSizeBytes = 4 * 1024 * 1024

	d, err := diskfs.Create(isoPath, isoSizeBytes, diskfs.SectorSize512)
	if err != nil {
		return "", fmt.Errorf("cloudinit: create iso: %w", err)
	}

	d.LogicalBlocksize = 2048
	fspec := disk.FilesystemSpec{
		Partition:   0,
		FSType:      filesystem.TypeISO9660,
		VolumeLabel: "cidata",
	}
	fs, err := d.CreateFilesystem(fspec)
	if err != nil {
		return "", fmt.Errorf("cloudinit: create filesystem: %w", err)
	}

	isoFS, ok := fs.(*iso9660.FileSystem)
	if !ok {
		return "", fmt.Errorf("cloudinit: unexpected filesystem type")
	}

	// Write meta-data
	metaData := spec.MetaData
	if metaData == "" {
		metaData = fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", spec.Hostname, spec.Hostname)
	}
	if err := writeISOFile(isoFS, "/meta-data", metaData); err != nil {
		return "", err
	}

	// Write user-data
	userData := spec.UserData
	if userData == "" {
		userData = "#cloud-config\n"
	}
	if err := writeISOFile(isoFS, "/user-data", userData); err != nil {
		return "", err
	}

	// Write network-config (optional)
	if spec.NetworkConfig != "" {
		if err := writeISOFile(isoFS, "/network-config", spec.NetworkConfig); err != nil {
			return "", err
		}
	}

	// Finalize ISO.
	if err := isoFS.Finalize(iso9660.FinalizeOptions{
		VolumeIdentifier: "cidata",
	}); err != nil {
		return "", fmt.Errorf("cloudinit: finalize iso: %w", err)
	}

	return isoPath, nil
}

func writeISOFile(fs *iso9660.FileSystem, path, content string) error {
	f, err := fs.OpenFile(path, os.O_CREATE|os.O_WRONLY)
	if err != nil {
		return fmt.Errorf("cloudinit: open %s: %w", path, err)
	}
	defer f.Close()
	if _, err := f.Write([]byte(content)); err != nil {
		return fmt.Errorf("cloudinit: write %s: %w", path, err)
	}
	return nil
}
