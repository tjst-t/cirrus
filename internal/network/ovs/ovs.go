package ovs

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/tjst-t/cirrus/internal/network"
)

type OVS struct {
	bridge  string
	localIP string

	mu          sync.Mutex
	nextTag     int
	vniToTag    map[int]int
	tunnelPorts map[string]string // peerAddr -> port name
}

func New(bridge, localIP string) *OVS {
	return &OVS{
		bridge:      bridge,
		localIP:     localIP,
		nextTag:     1,
		vniToTag:    make(map[int]int),
		tunnelPorts: make(map[string]string),
	}
}

func (o *OVS) InitBridge(ctx context.Context) error {
	return o.run(ctx, "ovs-vsctl", "--may-exist", "add-br", o.bridge)
}

func (o *OVS) AddTunnel(ctx context.Context, peerAddr string, peerName string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	portName := "vxlan-" + peerName
	o.tunnelPorts[peerAddr] = portName

	return o.run(ctx, "ovs-vsctl", "--may-exist", "add-port", o.bridge, portName,
		"--", "set", "interface", portName,
		"type=vxlan",
		"options:remote_ip="+peerAddr,
		"options:key=flow")
}

func (o *OVS) RemoveTunnel(ctx context.Context, peerAddr string, _ string) error {
	o.mu.Lock()
	portName, ok := o.tunnelPorts[peerAddr]
	if ok {
		delete(o.tunnelPorts, peerAddr)
	}
	o.mu.Unlock()
	if !ok {
		return nil
	}
	return o.run(ctx, "ovs-vsctl", "--if-exists", "del-port", o.bridge, portName)
}

func (o *OVS) AttachPort(ctx context.Context, port network.PortConfig) error {
	o.mu.Lock()
	tag, ok := o.vniToTag[port.VNI]
	if !ok {
		tag = o.nextTag
		o.nextTag++
		o.vniToTag[port.VNI] = tag
	}
	o.mu.Unlock()

	// Set the VLAN tag on the tap device (created by libvirt)
	tapName := "tap-" + port.ID[:8]
	if err := o.run(ctx, "ovs-vsctl", "set", "port", tapName, "tag="+strconv.Itoa(tag)); err != nil {
		return fmt.Errorf("set port tag: %w", err)
	}

	return o.EnsureFlows(ctx, port.VNI, tag)
}

func (o *OVS) DetachPort(ctx context.Context, portID string) error {
	tapName := "tap-" + portID[:8]
	return o.run(ctx, "ovs-vsctl", "--if-exists", "del-port", o.bridge, tapName)
}

func (o *OVS) EnsureFlows(ctx context.Context, vni int, localTag int) error {
	// Ingress: VXLAN → local VLAN tag
	ingressFlow := fmt.Sprintf("table=0,tun_id=%d,actions=mod_vlan_vid:%d,resubmit(,1)", vni, localTag)
	if err := o.run(ctx, "ovs-ofctl", "add-flow", o.bridge, ingressFlow); err != nil {
		return fmt.Errorf("add ingress flow: %w", err)
	}

	// Egress: local VLAN tag → VXLAN
	for _, portName := range o.tunnelPorts {
		egressFlow := fmt.Sprintf("table=2,dl_vlan=%d,actions=strip_vlan,set_tunnel:%d,output:%s", localTag, vni, portName)
		if err := o.run(ctx, "ovs-ofctl", "add-flow", o.bridge, egressFlow); err != nil {
			return fmt.Errorf("add egress flow: %w", err)
		}
	}
	return nil
}

func (o *OVS) RemoveFlows(ctx context.Context, vni int) error {
	o.mu.Lock()
	tag, ok := o.vniToTag[vni]
	if ok {
		delete(o.vniToTag, vni)
	}
	o.mu.Unlock()
	if !ok {
		return nil
	}

	flow := fmt.Sprintf("tun_id=%d", vni)
	if err := o.run(ctx, "ovs-ofctl", "del-flows", o.bridge, flow); err != nil {
		return err
	}
	flow = fmt.Sprintf("dl_vlan=%d", tag)
	return o.run(ctx, "ovs-ofctl", "del-flows", o.bridge, flow)
}

func (o *OVS) ListPorts(ctx context.Context) ([]network.PortConfig, error) {
	out, err := exec.CommandContext(ctx, "ovs-vsctl", "list-ports", o.bridge).Output()
	if err != nil {
		return nil, err
	}
	var ports []network.PortConfig
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" || strings.HasPrefix(line, "vxlan-") {
			continue
		}
		ports = append(ports, network.PortConfig{ID: line})
	}
	return ports, nil
}

func (o *OVS) run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s %s: %s: %w", name, strings.Join(args, " "), string(out), err)
	}
	return nil
}
