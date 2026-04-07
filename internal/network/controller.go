package network

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	pb "github.com/tjst-t/cirrus/proto/networkpb"
)

// StateController computes HostNetworkState for each host.
type StateController struct {
	pool       *pgxpool.Pool
	logger     *slog.Logger
	secretsKey []byte // AES-GCM key for decrypting WireGuard private keys
}

// NewStateController creates a new StateController.
func NewStateController(pool *pgxpool.Pool, logger *slog.Logger) *StateController {
	return &StateController{pool: pool, logger: logger}
}

// WithSecretsKey sets the AES-GCM key used to decrypt WireGuard private keys
// before delivering them to GW workers.
func (sc *StateController) WithSecretsKey(key []byte) *StateController {
	sc.secretsKey = key
	return sc
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

	// Check if this host is a gateway node before potentially returning early.
	gw, err := sc.getGatewayNodeForHost(ctx, hostID)
	if err != nil {
		return nil, fmt.Errorf("compute state: gw node: %w", err)
	}

	if len(networkIDs) == 0 && gw == nil {
		return &pb.HostNetworkState{}, nil
	}

	// For gateway hosts, also collect network IDs from ingress/egress rules
	// so that remote port routing (for DNAT forwarding) is included even when
	// the gateway host has no local VMs.
	if gw != nil && len(networkIDs) == 0 {
		gwNetIDs, err := sc.getGatewayNetworkIDs(ctx, gw.ID)
		if err != nil {
			return nil, fmt.Errorf("compute state: gw network ids: %w", err)
		}
		for _, id := range gwNetIDs {
			networkIDs[id] = true
		}
	}

	var policies []*pb.PolicyRule
	var remotePorts []*pb.RemotePort
	var dnsRecords []*pb.DnsRecord

	if len(networkIDs) > 0 {
		netIDs := make([]uuid.UUID, 0, len(networkIDs))
		for id := range networkIDs {
			netIDs = append(netIDs, id)
		}

		policies, err = sc.getPolicies(ctx, netIDs)
		if err != nil {
			return nil, fmt.Errorf("compute state: policies: %w", err)
		}

		remotePorts, err = sc.getRemotePorts(ctx, netIDs, hostID)
		if err != nil {
			return nil, fmt.Errorf("compute state: remote ports: %w", err)
		}

		dnsRecords, err = sc.getDNSRecords(ctx, netIDs)
		if err != nil {
			return nil, fmt.Errorf("compute state: dns records: %w", err)
		}
	}

	var egressRules []*pb.EgressRule
	var ingressRules []*pb.IngressRule
	var gatewayInfo *pb.GatewayInfo
	if gw != nil {
		gatewayInfo = &pb.GatewayInfo{ExternalIp: gw.ExternalIP, InternalIp: gw.InternalIP}

		egressRules, err = sc.computeEgressRules(ctx, gw)
		if err != nil {
			return nil, fmt.Errorf("compute state: egress rules: %w", err)
		}

		ingressRules, err = sc.computeIngressRules(ctx, gw)
		if err != nil {
			return nil, fmt.Errorf("compute state: ingress rules: %w", err)
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
		SELECT gn.id, gn.host_id, host(gn.external_ip), host(gn.internal_ip), gn.uplink_port, gn.status, gn.created_at
		FROM gateway_nodes gn
		JOIN hosts h ON h.id = gn.host_id
		WHERE h.id = $1 AND 'gateway' = ANY(h.node_roles) AND gn.status = 'active'
		LIMIT 1
	`, hostID).Scan(&gw.ID, &gw.HostID, &gw.ExternalIP, &gw.InternalIP, &gw.UplinkPort, &gw.Status, &gw.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &gw, nil
}

// getGatewayNetworkIDs returns the network IDs of all networks that have ingress or
// egress rules assigned to this gateway node. Used to ensure remote port routing
// is included for gateway hosts that have no local VMs.
func (sc *StateController) getGatewayNetworkIDs(ctx context.Context, gwID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := sc.pool.Query(ctx, `
		SELECT DISTINCT n.id
		FROM networks n
		WHERE n.gateway_node_id = $1
	`, gwID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// computeIngressRules returns all IngressRules for the networks assigned to this GW node.
// gw must be non-nil; callers must check before calling.
func (sc *StateController) computeIngressRules(ctx context.Context, gw *GatewayNode) ([]*pb.IngressRule, error) {
	// LEFT JOIN l4lb_backend_health to avoid N+1 queries; health rows are NULL for direct_ip.
	rows, err := sc.pool.Query(ctx, `
		SELECT i.id, i.network_id, i.type, host(i.public_ip), i.config,
		       h.vm_id::TEXT, h.healthy
		FROM ingresses i
		JOIN networks n ON n.id = i.network_id
		LEFT JOIN l4lb_backend_health h ON h.ingress_id = i.id
		WHERE n.gateway_node_id = $1 AND i.type = ANY($2)
		ORDER BY i.id
	`, gw.ID, []string{IngressTypeDirectIP, IngressTypeL4LB})
	if err != nil {
		return nil, fmt.Errorf("compute ingress: query: %w", err)
	}
	defer rows.Close()

	// Accumulate health rows per ingress, then build rules.
	type ingressRow struct {
		id        uuid.UUID
		networkID uuid.UUID
		iType     string
		publicIP  string
		config    []byte
	}
	var (
		ingressOrder []ingressRow
		healthMaps   = make(map[uuid.UUID]map[string]bool) // ingress_id → vm_id → healthy
		seenIDs      = make(map[uuid.UUID]bool)
	)

	for rows.Next() {
		var iID, iNetID uuid.UUID
		var iType, publicIP string
		var configJSON []byte
		var vmIDStr *string
		var healthy *bool
		if err := rows.Scan(&iID, &iNetID, &iType, &publicIP, &configJSON, &vmIDStr, &healthy); err != nil {
			return nil, fmt.Errorf("compute ingress: scan: %w", err)
		}
		if !seenIDs[iID] {
			seenIDs[iID] = true
			ingressOrder = append(ingressOrder, ingressRow{iID, iNetID, iType, publicIP, configJSON})
		}
		if vmIDStr != nil && healthy != nil {
			if healthMaps[iID] == nil {
				healthMaps[iID] = make(map[string]bool)
			}
			healthMaps[iID][*vmIDStr] = *healthy
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("compute ingress: rows: %w", err)
	}

	var ingressRules []*pb.IngressRule
	for _, ir := range ingressOrder {
		rule := &pb.IngressRule{
			IngressId: ir.id.String(),
			NetworkId: ir.networkID.String(),
			Type:      ir.iType,
			PublicIp:  ir.publicIP,
		}

		switch ir.iType {
		case IngressTypeDirectIP:
			var cfg IngressConfig
			if err := json.Unmarshal(ir.config, &cfg); err != nil {
				return nil, fmt.Errorf("compute ingress: unmarshal direct_ip config: %w", err)
			}
			rule.TargetIp = cfg.TargetIP

		case IngressTypeL4LB:
			var wrapper struct {
				L4LB *L4LBConfig `json:"l4lb"`
			}
			if err := json.Unmarshal(ir.config, &wrapper); err != nil {
				return nil, fmt.Errorf("compute ingress: unmarshal l4lb config: %w", err)
			}
			if wrapper.L4LB == nil {
				sc.logger.Warn("l4lb ingress has nil config, skipping", "ingress_id", ir.id)
				continue
			}
			cfg := wrapper.L4LB
			healthMap := healthMaps[ir.id]

			rule.ListenerPort = int32(cfg.ListenerPort)
			rule.Protocol = cfg.Protocol
			rule.SessionAffinity = cfg.SessionAffinity

			for _, b := range cfg.Backends {
				healthy := b.Healthy
				if h, ok := healthMap[b.VMID]; ok {
					healthy = h
				}
				if !healthy {
					continue
				}
				rule.Backends = append(rule.Backends, &pb.L4LBBackend{
					VmId:    b.VMID,
					Ip:      b.IP,
					Port:    int32(b.Port),
					Weight:  int32(b.Weight),
					Healthy: true,
				})
			}

			if len(rule.Backends) == 0 {
				sc.logger.Warn("l4lb ingress has no healthy backends, skipping", "ingress_id", ir.id)
				continue
			}
		}

		ingressRules = append(ingressRules, rule)
	}
	return ingressRules, nil
}

// computeEgressRules returns all EgressRules for the networks assigned to this GW node.
// gw must be non-nil; callers must check before calling.
func (sc *StateController) computeEgressRules(ctx context.Context, gw *GatewayNode) ([]*pb.EgressRule, error) {
	rows, err := sc.pool.Query(ctx, `
		SELECT e.id, e.network_id, e.type, e.config, n.cidr::TEXT
		FROM egresses e
		JOIN networks n ON n.id = e.network_id
		WHERE n.gateway_node_id = $1 AND e.type = ANY($2)
	`, gw.ID, []string{EgressTypeNATGateway, EgressTypeVPNIPsec, EgressTypeVPNWireGuard, EgressTypeDirectConnect})
	if err != nil {
		return nil, fmt.Errorf("compute egress: query: %w", err)
	}
	defer rows.Close()

	var egressRules []*pb.EgressRule
	for rows.Next() {
		var eID, eNetID uuid.UUID
		var eType, cidr string
		var configJSON []byte
		if err := rows.Scan(&eID, &eNetID, &eType, &configJSON, &cidr); err != nil {
			return nil, fmt.Errorf("compute egress: scan: %w", err)
		}
		var cfg EgressConfig
		if err := json.Unmarshal(configJSON, &cfg); err != nil {
			return nil, fmt.Errorf("compute egress: unmarshal config: %w", err)
		}

		rule := &pb.EgressRule{
			EgressId:    eID.String(),
			NetworkId:   eNetID.String(),
			Type:        eType,
			NetworkCidr: cidr,
		}

		switch eType {
		case EgressTypeNATGateway:
			rule.PublicIp = cfg.PublicIP
		case EgressTypeVPNIPsec:
			if cfg.VPNIPsec != nil {
				psk := cfg.VPNIPsec.PreSharedKey // may be empty if encrypted
				if cfg.VPNIPsec.PreSharedKeyEnc != "" && len(sc.secretsKey) > 0 {
					pskBytes, ok := sc.decryptSecret(eID.String(), "ipsec psk", cfg.VPNIPsec.PreSharedKeyEnc)
					if !ok {
						continue
					}
					psk = string(pskBytes)
				}
				rule.VpnIpsec = &pb.VPNIPsecConfig{
					PeerIp:       cfg.VPNIPsec.PeerIP,
					PreSharedKey: psk,
					LocalCidr:    cfg.VPNIPsec.LocalCIDR,
					RemoteCidr:   cfg.VPNIPsec.RemoteCIDR,
				}
			}
		case EgressTypeVPNWireGuard:
			if cfg.VPNWireGuard != nil {
				wgRule := &pb.VPNWireGuardConfig{
					PublicKey:     cfg.VPNWireGuard.PublicKey,
					PeerPublicKey: cfg.VPNWireGuard.PeerPublicKey,
					PeerEndpoint:  cfg.VPNWireGuard.PeerEndpoint,
					AllowedIps:    cfg.VPNWireGuard.AllowedIPs,
					ListenPort:    int32(cfg.VPNWireGuard.ListenPort),
				}
				// Decrypt the private key to deliver to the GW worker.
				if cfg.VPNWireGuard.PrivateKeyEnc != "" && len(sc.secretsKey) > 0 {
					privBytes, ok := sc.decryptSecret(eID.String(), "wireguard private key", cfg.VPNWireGuard.PrivateKeyEnc)
					if !ok {
						continue
					}
					wgRule.PrivateKey = encodeBase64(privBytes)
				}
				rule.VpnWireguard = wgRule
			}
		case EgressTypeDirectConnect:
			if cfg.DirectConnect != nil {
				// Use the stored uplink_port (auto-populated from GW node at create time);
				// fall back to the GW node's current uplink_port so it stays up-to-date.
				uplinkPort := cfg.DirectConnect.UplinkPort
				if uplinkPort == "" {
					uplinkPort = gw.UplinkPort
				}
				rule.DirectConnect = &pb.DirectConnectConfig{
					VlanId:     int32(cfg.DirectConnect.VLANID),
					UplinkPort: uplinkPort,
				}
			}
		}

		egressRules = append(egressRules, rule)
	}
	return egressRules, rows.Err()
}

// decryptSecret decodes a base64-encoded ciphertext and decrypts it with AES-GCM.
// Returns the plaintext and true on success; logs a warning and returns nil, false on any error.
// egressID and label are used solely for structured log context.
func (sc *StateController) decryptSecret(egressID, label, enc string) ([]byte, bool) {
	raw, err := decodeBase64(enc)
	if err != nil {
		sc.logger.Warn("compute egress: decode "+label+" failed, skipping rule", "egress_id", egressID, "error", err)
		return nil, false
	}
	plain, err := DecryptAESGCM(sc.secretsKey, raw)
	if err != nil {
		sc.logger.Warn("compute egress: decrypt "+label+" failed, skipping rule", "egress_id", egressID, "error", err)
		return nil, false
	}
	return plain, true
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

