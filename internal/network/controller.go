package network

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	pb "github.com/tjst-t/cirrus/proto/networkpb"
)

// StateController computes HostNetworkState for each host.
type StateController struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewStateController creates a new StateController.
func NewStateController(pool *pgxpool.Pool, logger *slog.Logger) *StateController {
	return &StateController{pool: pool, logger: logger}
}

// portRow holds a joined port+network+group row.
type portRow struct {
	PortID      uuid.UUID
	VMID        uuid.UUID
	VMName      string
	NetworkID   uuid.UUID
	NetworkName string
	GroupID     uuid.UUID
	GroupName   string
	MACAddress  string
	IPAddress   string
	HostID      uuid.UUID
	VNI         int32
}

// ComputeHostNetworkState builds the full HostNetworkState for a single host.
func (sc *StateController) ComputeHostNetworkState(ctx context.Context, hostID uuid.UUID) (*pb.HostNetworkState, error) {
	// 1. Local ports on this host
	localPorts, networkIDs, err := sc.getLocalPorts(ctx, hostID)
	if err != nil {
		return nil, fmt.Errorf("compute state: local ports: %w", err)
	}

	if len(networkIDs) == 0 {
		return &pb.HostNetworkState{}, nil
	}

	netIDs := make([]uuid.UUID, 0, len(networkIDs))
	for id := range networkIDs {
		netIDs = append(netIDs, id)
	}

	// 2-4 in parallel could be optimized, but sequential is fine for now
	policies, err := sc.getPolicies(ctx, netIDs)
	if err != nil {
		return nil, fmt.Errorf("compute state: policies: %w", err)
	}

	remotePorts, err := sc.getRemotePorts(ctx, netIDs, hostID)
	if err != nil {
		return nil, fmt.Errorf("compute state: remote ports: %w", err)
	}

	dnsRecords, err := sc.getDNSRecords(ctx, netIDs)
	if err != nil {
		return nil, fmt.Errorf("compute state: dns records: %w", err)
	}

	egressRules, gatewayInfo, err := sc.computeEgressRules(ctx, hostID)
	if err != nil {
		return nil, fmt.Errorf("compute state: egress rules: %w", err)
	}

	ingressRules, err := sc.computeIngressRules(ctx, hostID)
	if err != nil {
		return nil, fmt.Errorf("compute state: ingress rules: %w", err)
	}

	// If egress didn't set gatewayInfo (parallel story), use ingress's computation.
	if gatewayInfo == nil && len(ingressRules) > 0 {
		gw, gwErr := sc.getGatewayNodeForHost(ctx, hostID)
		if gwErr == nil && gw != nil {
			gatewayInfo = &pb.GatewayInfo{
				ExternalIp: gw.ExternalIP,
				InternalIp: gw.InternalIP,
			}
		}
	}

	return &pb.HostNetworkState{
		Ports:        localPorts,
		Policies:     policies,
		RemotePorts:  remotePorts,
		DnsRecords:   dnsRecords,
		EgressRules:  egressRules,
		IngressRules: ingressRules,
		GatewayInfo:  gatewayInfo,
	}, nil
}

func (sc *StateController) getLocalPorts(ctx context.Context, hostID uuid.UUID) ([]*pb.PortState, map[uuid.UUID]bool, error) {
	rows, err := sc.pool.Query(ctx, `
		SELECT p.id, p.vm_id, p.vm_name, p.network_id, n.name, COALESCE(p.group_id, '00000000-0000-0000-0000-000000000000'),
		       COALESCE(g.name, ''), p.mac_address::TEXT, host(p.ip_address), p.host_id, n.vni
		FROM ports p
		JOIN networks n ON p.network_id = n.id
		LEFT JOIN groups g ON p.group_id = g.id
		WHERE p.host_id = $1 AND p.status = 'active'
	`, hostID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var ports []*pb.PortState
	networkIDs := make(map[uuid.UUID]bool)

	for rows.Next() {
		var r portRow
		if err := rows.Scan(&r.PortID, &r.VMID, &r.VMName, &r.NetworkID, &r.NetworkName,
			&r.GroupID, &r.GroupName, &r.MACAddress, &r.IPAddress, &r.HostID, &r.VNI); err != nil {
			return nil, nil, err
		}

		gwIP := gatewayIPFromVM(r.IPAddress)

		ports = append(ports, &pb.PortState{
			PortId:      r.PortID.String(),
			VmId:        r.VMID.String(),
			VmName:      r.VMName,
			NetworkId:   r.NetworkID.String(),
			NetworkName: r.NetworkName,
			GroupId:     r.GroupID.String(),
			GroupName:   r.GroupName,
			MacAddress:  r.MACAddress,
			IpAddress:   r.IPAddress,
			GatewayIp:   gwIP,
			Vni:         r.VNI,
		})
		networkIDs[r.NetworkID] = true
	}
	return ports, networkIDs, rows.Err()
}

func (sc *StateController) getPolicies(ctx context.Context, networkIDs []uuid.UUID) ([]*pb.PolicyRule, error) {
	rows, err := sc.pool.Query(ctx, `
		SELECT id, network_id, src_group_id, dst_group_id, protocol, dst_port, priority, action
		FROM policies
		WHERE network_id = ANY($1)
	`, networkIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []*pb.PolicyRule
	for rows.Next() {
		var id, netID, srcGroup, dstGroup uuid.UUID
		var protocol, action string
		var dstPort *int
		var priority int

		if err := rows.Scan(&id, &netID, &srcGroup, &dstGroup, &protocol, &dstPort, &priority, &action); err != nil {
			return nil, err
		}

		var port int32
		if dstPort != nil {
			port = int32(*dstPort)
		}

		policies = append(policies, &pb.PolicyRule{
			PolicyId:   id.String(),
			NetworkId:  netID.String(),
			SrcGroupId: srcGroup.String(),
			DstGroupId: dstGroup.String(),
			Protocol:   protocol,
			DstPort:    port,
			Priority:   int32(priority),
			Action:     action,
		})
	}
	return policies, rows.Err()
}

func (sc *StateController) getRemotePorts(ctx context.Context, networkIDs []uuid.UUID, excludeHostID uuid.UUID) ([]*pb.RemotePort, error) {
	rows, err := sc.pool.Query(ctx, `
		SELECT p.network_id, COALESCE(p.group_id, '00000000-0000-0000-0000-000000000000'),
		       host(p.ip_address), h.fabric_ip, n.vni
		FROM ports p
		JOIN hosts h ON p.host_id = h.id
		JOIN networks n ON p.network_id = n.id
		WHERE p.network_id = ANY($1)
		  AND p.host_id != $2
		  AND p.status = 'active'
	`, networkIDs, excludeHostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var remotePorts []*pb.RemotePort
	for rows.Next() {
		var netID, groupID uuid.UUID
		var ipAddr, hostAddr string
		var vni int32

		if err := rows.Scan(&netID, &groupID, &ipAddr, &hostAddr, &vni); err != nil {
			return nil, err
		}

		remotePorts = append(remotePorts, &pb.RemotePort{
			NetworkId: netID.String(),
			GroupId:   groupID.String(),
			IpAddress: ipAddr,
			HostIp:    hostAddr,
			Vni:       vni,
		})
	}
	return remotePorts, rows.Err()
}

func (sc *StateController) getDNSRecords(ctx context.Context, networkIDs []uuid.UUID) ([]*pb.DnsRecord, error) {
	rows, err := sc.pool.Query(ctx, `
		SELECT p.vm_name, COALESCE(g.name, 'default'), n.name, host(p.ip_address), p.network_id
		FROM ports p
		JOIN networks n ON p.network_id = n.id
		LEFT JOIN groups g ON p.group_id = g.id
		WHERE p.network_id = ANY($1) AND p.status = 'active'
	`, networkIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*pb.DnsRecord
	for rows.Next() {
		var vmName, groupName, netName, ipAddr string
		var netID uuid.UUID

		if err := rows.Scan(&vmName, &groupName, &netName, &ipAddr, &netID); err != nil {
			return nil, err
		}

		if vmName == "" {
			continue
		}

		// Per-VM record: vm.group.network.internal
		records = append(records, &pb.DnsRecord{
			Name:      fmt.Sprintf("%s.%s.%s.internal", vmName, groupName, netName),
			Ip:        ipAddr,
			NetworkId: netID.String(),
		})
	}
	return records, rows.Err()
}

// getGatewayNodeForHost returns the GatewayNode for this host if it has the
// 'gateway' role. Returns nil, nil if the host is not a GW node.
func (sc *StateController) getGatewayNodeForHost(ctx context.Context, hostID uuid.UUID) (*GatewayNode, error) {
	var gw GatewayNode
	err := sc.pool.QueryRow(ctx, `
		SELECT gn.id, gn.host_id, host(gn.external_ip), host(gn.internal_ip), gn.status, gn.created_at
		FROM gateway_nodes gn
		JOIN hosts h ON h.id = gn.host_id
		WHERE h.id = $1 AND 'gateway' = ANY(h.node_roles) AND gn.status = 'active'
		LIMIT 1
	`, hostID).Scan(&gw.ID, &gw.HostID, &gw.ExternalIP, &gw.InternalIP, &gw.Status, &gw.CreatedAt)
	if err != nil {
		// Not a GW node — not an error condition
		return nil, nil
	}
	return &gw, nil
}

// computeIngressRules determines if this host is a GW node and, if so, returns
// all IngressRules for the networks assigned to it.
func (sc *StateController) computeIngressRules(ctx context.Context, hostID uuid.UUID) ([]*pb.IngressRule, error) {
	gw, err := sc.getGatewayNodeForHost(ctx, hostID)
	if err != nil {
		return nil, fmt.Errorf("compute ingress: get gw node: %w", err)
	}
	if gw == nil {
		// Not a GW node
		return nil, nil
	}

	// Find all networks assigned to this GW node
	netRows, err := sc.pool.Query(ctx, `
		SELECT n.id FROM networks n WHERE n.gateway_node_id = $1
	`, gw.ID)
	if err != nil {
		return nil, fmt.Errorf("compute ingress: list networks: %w", err)
	}
	defer netRows.Close()

	var netIDs []uuid.UUID
	for netRows.Next() {
		var id uuid.UUID
		if err := netRows.Scan(&id); err != nil {
			return nil, fmt.Errorf("compute ingress: scan network: %w", err)
		}
		netIDs = append(netIDs, id)
	}
	if err := netRows.Err(); err != nil {
		return nil, fmt.Errorf("compute ingress: iter networks: %w", err)
	}

	var ingressRules []*pb.IngressRule
	for _, netID := range netIDs {
		iRows, err := sc.pool.Query(ctx, `
			SELECT i.id, i.network_id, i.type, host(i.public_ip), i.config
			FROM ingresses i
			WHERE i.network_id = $1 AND i.type = 'direct_ip'
		`, netID)
		if err != nil {
			return nil, fmt.Errorf("compute ingress: list ingresses for network %s: %w", netID, err)
		}

		for iRows.Next() {
			var iID, iNetID uuid.UUID
			var iType, publicIP string
			var configJSON []byte
			if err := iRows.Scan(&iID, &iNetID, &iType, &publicIP, &configJSON); err != nil {
				iRows.Close()
				return nil, fmt.Errorf("compute ingress: scan ingress: %w", err)
			}
			var cfg IngressConfig
			if err := json.Unmarshal(configJSON, &cfg); err != nil {
				iRows.Close()
				return nil, fmt.Errorf("compute ingress: unmarshal config: %w", err)
			}
			ingressRules = append(ingressRules, &pb.IngressRule{
				IngressId: iID.String(),
				NetworkId: iNetID.String(),
				Type:      iType,
				PublicIp:  publicIP,
				TargetIp:  cfg.TargetIP,
			})
		}
		iRows.Close()
		if err := iRows.Err(); err != nil {
			return nil, fmt.Errorf("compute ingress: iter ingresses: %w", err)
		}
	}

	return ingressRules, nil
}

// computeEgressRules determines if this host is a GW node and, if so, returns
// all EgressRules and GatewayInfo for the networks assigned to it.
func (sc *StateController) computeEgressRules(ctx context.Context, hostID uuid.UUID) ([]*pb.EgressRule, *pb.GatewayInfo, error) {
	// Check if this host is an active GW node
	var gwID uuid.UUID
	var externalIP, internalIP string
	err := sc.pool.QueryRow(ctx, `
		SELECT gn.id, host(gn.external_ip), host(gn.internal_ip)
		FROM gateway_nodes gn
		JOIN hosts h ON h.id = gn.host_id
		WHERE h.id = $1 AND 'gateway' = ANY(h.node_roles) AND gn.status = 'active'
		LIMIT 1
	`, hostID).Scan(&gwID, &externalIP, &internalIP)
	if err != nil {
		// Not a GW node (or no rows) — not an error condition
		return nil, nil, nil
	}

	gatewayInfo := &pb.GatewayInfo{
		ExternalIp: externalIP,
		InternalIp: internalIP,
	}

	// Find all networks assigned to this GW node
	netRows, err := sc.pool.Query(ctx, `
		SELECT n.id, n.cidr::TEXT FROM networks n WHERE n.gateway_node_id = $1
	`, gwID)
	if err != nil {
		return nil, nil, fmt.Errorf("compute egress: list networks: %w", err)
	}
	defer netRows.Close()

	type netInfo struct {
		id   uuid.UUID
		cidr string
	}
	var nets []netInfo
	for netRows.Next() {
		var ni netInfo
		if err := netRows.Scan(&ni.id, &ni.cidr); err != nil {
			return nil, nil, fmt.Errorf("compute egress: scan network: %w", err)
		}
		nets = append(nets, ni)
	}
	if err := netRows.Err(); err != nil {
		return nil, nil, fmt.Errorf("compute egress: iter networks: %w", err)
	}

	var egressRules []*pb.EgressRule
	for _, ni := range nets {
		eRows, err := sc.pool.Query(ctx, `
			SELECT e.id, e.type, e.config FROM egresses e
			WHERE e.network_id = $1 AND e.type = 'nat_gateway'
		`, ni.id)
		if err != nil {
			return nil, nil, fmt.Errorf("compute egress: list egresses for network %s: %w", ni.id, err)
		}

		for eRows.Next() {
			var eID uuid.UUID
			var eType string
			var configJSON []byte
			if err := eRows.Scan(&eID, &eType, &configJSON); err != nil {
				eRows.Close()
				return nil, nil, fmt.Errorf("compute egress: scan egress: %w", err)
			}
			var cfg EgressConfig
			if err := json.Unmarshal(configJSON, &cfg); err != nil {
				eRows.Close()
				return nil, nil, fmt.Errorf("compute egress: unmarshal config: %w", err)
			}
			egressRules = append(egressRules, &pb.EgressRule{
				EgressId:    eID.String(),
				NetworkId:   ni.id.String(),
				Type:        eType,
				PublicIp:    cfg.PublicIP,
				NetworkCidr: ni.cidr,
			})
		}
		eRows.Close()
		if err := eRows.Err(); err != nil {
			return nil, nil, fmt.Errorf("compute egress: iter egresses: %w", err)
		}
	}

	return egressRules, gatewayInfo, nil
}

// gatewayIPFromVM computes the gateway IP (.2) from the VM IP (.1) in a /30 block.
// Given "100.64.0.1", returns "100.64.0.2".
func gatewayIPFromVM(vmIP string) string {
	ip := net.ParseIP(vmIP)
	if ip == nil {
		return ""
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return ""
	}
	// In a /30: .0=network, .1=VM, .2=gateway, .3=broadcast
	// VM is always .1, gateway is .1 + 1 = .2
	ip4[3] = (ip4[3] & 0xFC) | 0x02
	return ip4.String()
}

