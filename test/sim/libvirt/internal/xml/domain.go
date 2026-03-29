// Package xml provides domain XML parsing for libvirt-sim.
package xml

import (
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"strings"
)

// DomainXML represents a parsed libvirt domain XML document.
type DomainXML struct {
	XMLName xml.Name       `xml:"domain"`
	Type    string         `xml:"type,attr"`
	Name    string         `xml:"name"`
	UUID    string         `xml:"uuid"`
	Memory  DomainMemory   `xml:"memory"`
	VCPU    int            `xml:"vcpu"`
	Devices DomainDevices  `xml:"devices"`
}

// DomainMemory represents the memory element with a unit attribute.
type DomainMemory struct {
	Unit  string `xml:"unit,attr"`
	Value int64  `xml:",chardata"`
}

// DomainDevices represents the devices section of domain XML.
type DomainDevices struct {
	Disks      []DomainDisk      `xml:"disk"`
	Interfaces []DomainInterface `xml:"interface"`
	HostDevs   []DomainHostDev   `xml:"hostdev"`
}

// DomainDisk represents a disk device.
type DomainDisk struct {
	Type   string          `xml:"type,attr"`
	Source DomainDiskSource `xml:"source"`
}

// DomainDiskSource represents a disk source.
type DomainDiskSource struct {
	Protocol string `xml:"protocol,attr,omitempty"`
	Name     string `xml:"name,attr,omitempty"`
}

// DomainInterface represents a network interface.
type DomainInterface struct {
	Type        string                    `xml:"type,attr"`
	Source      DomainInterfaceSource     `xml:"source"`
	Target      DomainInterfaceTarget     `xml:"target"`
	VirtualPort *DomainInterfaceVirtPort  `xml:"virtualport"`
}

// DomainInterfaceSource represents the interface source (e.g., bridge name).
type DomainInterfaceSource struct {
	Bridge string `xml:"bridge,attr,omitempty"`
}

// DomainInterfaceTarget represents interface target.
type DomainInterfaceTarget struct {
	Dev string `xml:"dev,attr"`
}

// DomainInterfaceVirtPort represents a virtualport element for OVS integration.
type DomainInterfaceVirtPort struct {
	Type       string                         `xml:"type,attr"`
	Parameters DomainInterfaceVirtPortParams   `xml:"parameters"`
}

// DomainInterfaceVirtPortParams holds virtualport parameters.
type DomainInterfaceVirtPortParams struct {
	InterfaceID string `xml:"interfaceid,attr,omitempty"`
}

// DomainHostDev represents a host device passthrough.
type DomainHostDev struct {
	Mode    string              `xml:"mode,attr"`
	Type    string              `xml:"type,attr"`
	Source  DomainHostDevSource `xml:"source"`
}

// DomainHostDevSource represents host device source.
type DomainHostDevSource struct {
	Address DomainHostDevAddress `xml:"address"`
}

// DomainHostDevAddress represents a PCI address.
type DomainHostDevAddress struct {
	Domain   string `xml:"domain,attr"`
	Bus      string `xml:"bus,attr"`
	Slot     string `xml:"slot,attr"`
	Function string `xml:"function,attr"`
}

// ParseDomainXML parses a libvirt domain XML string.
func ParseDomainXML(data string) (*DomainXML, error) {
	var dom DomainXML
	if err := xml.Unmarshal([]byte(data), &dom); err != nil {
		return nil, fmt.Errorf("parse domain XML: %w", err)
	}
	return &dom, nil
}

// MemoryKiB returns the memory in KiB regardless of the original unit.
func (d *DomainXML) MemoryKiB() int64 {
	switch strings.ToLower(d.Memory.Unit) {
	case "kib", "k":
		return d.Memory.Value
	case "mib", "m":
		return d.Memory.Value * 1024
	case "gib", "g":
		return d.Memory.Value * 1024 * 1024
	case "tib", "t":
		return d.Memory.Value * 1024 * 1024 * 1024
	case "b", "bytes":
		return d.Memory.Value / 1024
	default:
		// Default is KiB
		return d.Memory.Value
	}
}

// InterfaceIDs returns the OVS interfaceid values from all virtualport-enabled interfaces.
func (d *DomainXML) InterfaceIDs() []string {
	var ids []string
	for _, iface := range d.Devices.Interfaces {
		if iface.VirtualPort != nil && iface.VirtualPort.Parameters.InterfaceID != "" {
			ids = append(ids, iface.VirtualPort.Parameters.InterfaceID)
		}
	}
	return ids
}

// ParseUUID parses a UUID string into a 16-byte array.
func ParseUUID(s string) ([16]byte, error) {
	var uuid [16]byte
	s = strings.ReplaceAll(s, "-", "")
	if len(s) != 32 {
		return uuid, fmt.Errorf("invalid UUID length: %d", len(s))
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return uuid, fmt.Errorf("invalid UUID hex: %w", err)
	}
	copy(uuid[:], b)
	return uuid, nil
}
