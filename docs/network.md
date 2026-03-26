# ネットワーク設計

## 基本方針

OVN一本。OVNは論理スイッチ、論理ルータ、ACL、DHCP、DNS、NATを内包。CirrusはOVN Northbound DBへの書き込みを通じてネットワークを制御し、データプレーンの複雑さはOVNに任せる。

## ユーザ向けリソース

### ネットワーク

テナントに所属するL2ブロードキャストドメイン。OVN論理スイッチに対応。

### サブネット

ネットワーク内のIPアドレス範囲。CIDR、ゲートウェイ、DHCPレンジ、DNSサーバ。OVN DHCP Optionsに変換。

### ポート

VMの仮想NIC。IPアドレス、MACアドレス、セキュリティグループの参照。OVN論理スイッチポートに対応。

### ルータ

テナント内のL3ルーティングおよび外部接続。OVN論理ルータに対応。

### セキュリティグループ

ポート単位のステートフルファイアウォール。デフォルト全拒否のホワイトリスト方式。OVN ACL+conntrackで実装。

- セキュリティグループ間の参照をサポート
- テナント作成時にデフォルトセキュリティグループが自動作成される
- 同一グループ内のVM間通信は許可、それ以外は全拒否

### フローティングIP

外部IPとVMポートの1:1マッピング。OVN論理ルータ上のDNATルールに変換。

## インフラ向けリソース

**外部ネットワーク** — 管理者が定義する物理ネットワークとの接続点。gateway chassisとの紐付け。

**プロバイダネットワーク** — 物理VLAN/EVPNファブリックとの直接マッピング。OVNのlocalnetポートで実装。

## OVNリソースへのマッピング

| Cirrusリソース | OVNリソース |
|----------------|-------------|
| ネットワーク | Logical Switch |
| サブネット | DHCP Options |
| ポート | Logical Switch Port |
| ルータ | Logical Router |
| セキュリティグループ | ACL + conntrack |
| フローティングIP | Logical Router DNAT Rule |
| 外部ネットワーク | Logical Switch + localnet port |
| プロバイダネットワーク | Logical Switch + localnet port |

## IPアドレス管理

初期はCirrus内蔵のIPAM。外部IPAM（NetBox、Infoblox等）連携はインターフェースだけ定義しておく。

```go
// internal/network/ipam/ipam.go
package ipam

type IPAM interface {
    AllocateIP(ctx context.Context, subnetID string) (net.IP, error)
    ReleaseIP(ctx context.Context, subnetID string, ip net.IP) error
    AllocateSubnet(ctx context.Context, pool string, prefixLen int) (*net.IPNet, error)
    ReleaseSubnet(ctx context.Context, subnet *net.IPNet) error
}
```

## 外部ネットワーク連携

**OVN内で完結** — テナント間ルーティング、NAT、フローティングIP。

**L3連携** — ovn-bgp-agentでOVNプレフィックスをBGPでEVPNファブリックにアドバタイズ。

**L2ブリッジング** — gateway chassisのlocalnetポートで物理VLAN/EVPNと直接接続。

**拠点間L2延伸** — OVN Interconnect (OVN-IC)で独立したOVNクラスタ間を接続。各クラスタの独立性を維持。

## OVNクラスタの運用

OVNクラスタ自体の管理はAWX経由のhook。CirrusはOVNのAPIクライアントであり、OVNクラスタの管理者ではない。各ホストのovn-controllerはホストプロファイルの一部として管理。

## スケジューラとの連携

ネットワークはオーバーレイのため、ストレージほど強い配置制約にはならない。ただしgateway chassisの配置（HA対応）とSR-IOV対応NICの有無はプレースメントに影響。帯域管理は初期では不要。
