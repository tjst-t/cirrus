package hypervisor

import (
	"bytes"
	"fmt"
	"text/template"
)

// domainXMLTemplate is a minimal libvirt domain XML template for KVM VMs.
var domainXMLTemplate = template.Must(template.New("domain").Parse(`<domain type='kvm'>
  <name>{{.Name}}</name>
{{- if .UUID}}
  <uuid>{{.UUID}}</uuid>
{{- end}}
  <memory unit='MiB'>{{.RAMMB}}</memory>
  <vcpu>{{.VCPUs}}</vcpu>
  <os>
    <type arch='x86_64' machine='pc-q35-7.2'>hvm</type>
    <boot dev='hd'/>
  </os>
  <features>
    <acpi/>
    <apic/>
  </features>
  <cpu mode='host-model'/>
  <devices>
    <emulator>/usr/bin/qemu-system-x86_64</emulator>
{{- range $i, $d := .Disks}}
    <disk type='block' device='disk'>
      <driver name='qemu' type='raw'/>
      <source dev='{{$d.DevicePath}}'/>
      <target dev='{{$d.TargetDev}}' bus='virtio'/>
    </disk>
{{- end}}
{{- if .CloudInitISOPath}}
    <disk type='file' device='cdrom'>
      <driver name='qemu' type='raw'/>
      <source file='{{.CloudInitISOPath}}'/>
      <target dev='sda' bus='sata'/>
      <readonly/>
    </disk>
{{- end}}
{{- range .Interfaces}}
    <interface type='bridge'>
      <source bridge='{{.BridgeName}}'/>
      <mac address='{{.MACAddress}}'/>
      <virtualport type='openvswitch'>
        <parameters interfaceid='{{.PortID}}'/>
      </virtualport>
      <model type='virtio'/>
    </interface>
{{- end}}
    <serial type='pty'>
      <target port='0'/>
    </serial>
    <console type='pty'>
      <target type='serial' port='0'/>
    </console>
  </devices>
</domain>`))

// BuildDomainXML generates a libvirt domain XML string from a VMSpec.
func BuildDomainXML(spec VMSpec) (string, error) {
	var buf bytes.Buffer
	if err := domainXMLTemplate.Execute(&buf, spec); err != nil {
		return "", fmt.Errorf("hypervisor: build domain xml: %w", err)
	}
	return buf.String(), nil
}
