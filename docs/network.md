# ネットワーク設計

## 全体像

テナントごとにVXLAN VNIで分離されたL2ネットワークを提供する。

```
Worker-01                          Worker-02
┌──────────────────────┐          ┌──────────────────────┐
│  VM-A (tenant-1)     │          │  VM-C (tenant-1)     │
│  10.100.0.5          │          │  10.100.0.7          │
│    │ tap-{vm-a}      │          │    │ tap-{vm-c}      │
│    │                 │          │    │                 │
│  ┌─▼───────────────┐ │          │  ┌─▼───────────────┐ │
│  │   br-int (OVS)  │ │          │  │   br-int (OVS)  │ │
│  │                  │ │          │  │                  │ │
│  │ tag VNI=100     │ │          │  │ tag VNI=100     │ │
│  │ tag VNI=200     │ │          │  │ tag VNI=200     │ │
│  └──────┬──────────┘ │          │  └──────┬──────────┘ │
│         │ vxlan-w02   │          │         │ vxlan-w01   │
└─────────┼────────────┘          └─────────┼────────────┘
          │       VXLAN tunnel              │
          └─────────────────────────────────┘
          (physical network: 192.168.1.0/24)
```

## interface

```go
// internal/network/provider.go
package network

type Provider interface {
    // ブリッジ初期化（worker起動時）
    InitBridge(ctx context.Context) error

    // トンネル管理
    AddTunnel(ctx context.Context, peerAddr string) error
    RemoveTunnel(ctx context.Context, peerAddr string) error

    // ポート操作（VM作成/削除時）
    AttachPort(ctx context.Context, port PortConfig) error
    DetachPort(ctx context.Context, portID string) error

    // フロー管理
    EnsureFlows(ctx context.Context, vni int, localTag int) error
    RemoveFlows(ctx context.Context, vni int) error

    // 状態取得（reconcile用）
    ListPorts(ctx context.Context) ([]PortConfig, error)
}

type PortConfig struct {
    ID        string
    TapDevice string
    MAC       string
    VNI       int
    LocalTag  int
}
```

## OVS構成

### ブリッジ作成（初回のみ）

```bash
ovs-vsctl add-br br-int
```

### VXLANトンネル（worker追加時にcontrollerが指示）

```bash
ovs-vsctl add-port br-int vxlan-w02 \
  -- set interface vxlan-w02 type=vxlan \
     options:remote_ip=192.168.1.12 \
     options:key=flow  # key=flow → フローでVNIを動的指定
```

### VMポート接続時

```bash
# tapデバイスはlibvirtが自動作成、OVS側でタグ設定
ovs-vsctl set port tap-{vm-id} tag=100  # ← ローカルVLANタグ
```

## VNI ↔ ローカルVLANタグのマッピング

OVSのVXLANで `key=flow` を使う場合、OpenFlowルールでローカルVLANタグ ↔ VNIの変換をする（Neutron OVS agentと同じ方式）。

```bash
# Ingress: VXLANから来たパケット → ローカルVLANタグ付与
ovs-ofctl add-flow br-int \
  "table=0,in_port=vxlan-w02,tun_id=100,actions=mod_vlan_vid:1,resubmit(,1)"

# Egress: ローカルVLANタグ → VXLANで送出
ovs-ofctl add-flow br-int \
  "table=2,dl_vlan=1,actions=strip_vlan,set_tunnel:100,output:vxlan-w02"
```

## ワーカーローカル状態

local_vlan_tagの割り当てはOVS自体から逆引き可能。バックエンド個別のDBは持たない。worker起動時にcontrollerからstate同期してreconcileする。

## Phase 1の制限

- テナントネットワーク内のVM間通信のみ
- 外部接続（Floating IP、NAT）はPhase 2
- DHCPはなし（cloud-initでstatic IP設定）
- VXLANフルメッシュ（数十台まではこれで十分）

## ゲートウェイ（Phase 2）

VMがインターネットに出る場合、controller上にnetwork namespace + iptables NATを置く方式が最もシンプル。
