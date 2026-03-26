package libvirt

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/tjst-t/cirrus/internal/compute"
)

type Driver struct {
	uri      string
	cloudDir string
}

func New(uri, cloudDir string) *Driver {
	if cloudDir == "" {
		cloudDir = "/var/lib/cirrus/cloud-init"
	}
	return &Driver{uri: uri, cloudDir: cloudDir}
}

var domainTmpl = template.Must(template.New("domain").Parse(`<domain type='kvm'>
  <name>cirrus-{{.ID}}</name>
  <uuid>{{.ID}}</uuid>
  <memory unit='MiB'>{{.RamMB}}</memory>
  <vcpu>{{.VCPUs}}</vcpu>
  <os>
    <type arch='x86_64'>hvm</type>
    <boot dev='hd'/>
  </os>
  <cpu mode='host-passthrough'/>
  <devices>
    <disk type='file' device='disk'>
      <driver name='qemu' type='{{.Disk.Format}}' discard='unmap'/>
      <source file='{{.Disk.Source}}'/>
      <target dev='vda' bus='virtio'/>
    </disk>
    <disk type='file' device='cdrom'>
      <source file='{{.CloudInitISO}}'/>
      <target dev='sda' bus='sata'/>
      <readonly/>
    </disk>
    {{- range .Ports}}
    <interface type='bridge'>
      <mac address='{{.MAC}}'/>
      <source bridge='br-int'/>
      <virtualport type='openvswitch'>
        <parameters interfaceid='{{.ID}}'/>
      </virtualport>
      <target dev='tap-{{slice .ID 0 8}}'/>
      <model type='virtio'/>
    </interface>
    {{- end}}
    <serial type='pty'/>
    <console type='pty'/>
    <graphics type='vnc' port='-1' autoport='yes' listen='0.0.0.0'/>
  </devices>
</domain>`))

type domainData struct {
	compute.VMSpec
	CloudInitISO string
}

func (d *Driver) CreateVM(ctx context.Context, spec compute.VMSpec) error {
	// Generate cloud-init ISO
	cisoPath, err := d.generateCloudInit(ctx, spec)
	if err != nil {
		return fmt.Errorf("generate cloud-init: %w", err)
	}

	// Generate domain XML
	var xmlBuf bytes.Buffer
	data := domainData{
		VMSpec:       spec,
		CloudInitISO: cisoPath,
	}
	if err := domainTmpl.Execute(&xmlBuf, data); err != nil {
		return fmt.Errorf("render domain xml: %w", err)
	}

	// Write XML to temp file
	xmlPath := filepath.Join(os.TempDir(), "cirrus-"+spec.ID+".xml")
	if err := os.WriteFile(xmlPath, xmlBuf.Bytes(), 0644); err != nil {
		return fmt.Errorf("write domain xml: %w", err)
	}
	defer os.Remove(xmlPath)

	// Define and start domain
	if err := d.virsh(ctx, "define", xmlPath); err != nil {
		return fmt.Errorf("virsh define: %w", err)
	}
	if err := d.virsh(ctx, "start", "cirrus-"+spec.ID); err != nil {
		return fmt.Errorf("virsh start: %w", err)
	}
	return nil
}

func (d *Driver) DeleteVM(ctx context.Context, vmID string) error {
	name := "cirrus-" + vmID
	// Try to destroy (force stop) first, ignore errors if already stopped
	_ = d.virsh(ctx, "destroy", name)
	// Undefine
	if err := d.virsh(ctx, "undefine", name); err != nil {
		return fmt.Errorf("virsh undefine: %w", err)
	}
	// Clean up cloud-init ISO
	os.Remove(filepath.Join(d.cloudDir, vmID+".iso"))
	return nil
}

func (d *Driver) StopVM(ctx context.Context, vmID string) error {
	return d.virsh(ctx, "shutdown", "cirrus-"+vmID)
}

func (d *Driver) StartVM(ctx context.Context, vmID string) error {
	return d.virsh(ctx, "start", "cirrus-"+vmID)
}

func (d *Driver) GetStatus(ctx context.Context, vmID string) (compute.VMStatus, error) {
	name := "cirrus-" + vmID
	out, err := exec.CommandContext(ctx, "virsh", "-c", d.uri, "domstate", name).Output()
	if err != nil {
		return compute.VMStatus{ID: vmID, Status: "not_found"}, nil
	}
	status := "unknown"
	switch s := string(bytes.TrimSpace(out)); s {
	case "running":
		status = "running"
	case "shut off":
		status = "shutoff"
	case "paused":
		status = "paused"
	default:
		status = s
	}
	return compute.VMStatus{ID: vmID, Status: status}, nil
}

func (d *Driver) ListVMs(ctx context.Context) ([]compute.VMStatus, error) {
	out, err := exec.CommandContext(ctx, "virsh", "-c", d.uri, "list", "--all", "--name").Output()
	if err != nil {
		return nil, fmt.Errorf("virsh list: %w", err)
	}

	var vms []compute.VMStatus
	for _, line := range bytes.Split(bytes.TrimSpace(out), []byte("\n")) {
		name := string(bytes.TrimSpace(line))
		if name == "" || len(name) <= 7 || name[:7] != "cirrus-" {
			continue
		}
		vmID := name[7:]
		st, _ := d.GetStatus(ctx, vmID)
		vms = append(vms, st)
	}
	return vms, nil
}

func (d *Driver) generateCloudInit(ctx context.Context, spec compute.VMSpec) (string, error) {
	if err := os.MkdirAll(d.cloudDir, 0755); err != nil {
		return "", err
	}

	tmpDir, err := os.MkdirTemp("", "cirrus-ci-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	// If CloudInit data is provided directly, use it
	if spec.CloudInit != nil {
		if err := os.WriteFile(filepath.Join(tmpDir, "user-data"), spec.CloudInit, 0644); err != nil {
			return "", err
		}
	} else {
		if err := os.WriteFile(filepath.Join(tmpDir, "user-data"), []byte("#cloud-config\n"), 0644); err != nil {
			return "", err
		}
	}

	// meta-data
	metaData := fmt.Sprintf("instance-id: %s\nlocal-hostname: %s\n", spec.ID, spec.Name)
	if err := os.WriteFile(filepath.Join(tmpDir, "meta-data"), []byte(metaData), 0644); err != nil {
		return "", err
	}

	isoPath := filepath.Join(d.cloudDir, spec.ID+".iso")
	cmd := exec.CommandContext(ctx, "genisoimage",
		"-output", isoPath,
		"-volid", "cidata",
		"-joliet", "-rock",
		filepath.Join(tmpDir, "user-data"),
		filepath.Join(tmpDir, "meta-data"))
	if out, err := cmd.CombinedOutput(); err != nil {
		// Try mkisofs as fallback
		cmd2 := exec.CommandContext(ctx, "mkisofs",
			"-output", isoPath,
			"-volid", "cidata",
			"-joliet", "-rock",
			filepath.Join(tmpDir, "user-data"),
			filepath.Join(tmpDir, "meta-data"))
		if out2, err2 := cmd2.CombinedOutput(); err2 != nil {
			return "", fmt.Errorf("genisoimage: %s, mkisofs: %s", string(out), string(out2))
		}
	}
	return isoPath, nil
}

func (d *Driver) virsh(ctx context.Context, args ...string) error {
	allArgs := append([]string{"-c", d.uri}, args...)
	cmd := exec.CommandContext(ctx, "virsh", allArgs...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("virsh %v: %s: %w", args, string(out), err)
	}
	return nil
}
