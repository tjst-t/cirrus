# ネットワーク設計

## 基本方針

OVNは使わない。OVS（Open vSwitch）をデータプレーンとして使用し、Cirrus自身がOVSコントローラとして機能する。AWS VPCのモデルを参考にしつつ、さらに抽象度を上げた独自のネットワークモデルを採用する。OVNのような汎用的な論理トポロジエンジンではなく、意図的に制約されたビルディングブロックの組み合わせでネットワークを表現する。

## 設計原則

- **IaaSに徹する**: Cirrusはネットワークプリミティブの提供に集中し、L7 LBやFW等のアプリケーション層は関知しない
- **VPCモデルの制約を活かす**: 任意の論理トポロジではなく、固定パターンのビルディングブロック
- **IPアドレスの隠蔽**: ユーザはIPやサブネットを意識せず、通信の意図（誰が誰と何で話せるか）だけを定義する
- **DNS駆動の通信**: VM間通信はすべてDNS名で行う
- **Cirrusのモジュールとして実装**: 独立したソリューションではなく、モジュラーモノリスの一部

---

## ネットワークモデル

### 基本概念

概念は4つだけで構成される。

| 概念 | 説明 |
|------|------|
| **Network** | 隔離の単位。テナントごとに複数持てる。独立したIPアドレス空間を持つ |
| **Group** | VMの集合。ポリシー適用とルーティングの単位 |
| **Policy** | Group間の通信許可ルール。デフォルト全拒否 |
| **Egress / Ingress** | 外部接続点。Internet、VPN、専用線、テナント間サービス公開 |

従来のネットワークモデルとの比較：

- サブネットという概念がない
- ルータという概念がない
- VLANを意識する必要がない
- IPアドレスはシステムが自動採番する

### 設計の根拠

AWS VPCの実績が示しているのは、大多数のワークロードはテナント隔離、通信制御、外部接続の3つがあれば動作するということ。テナント内に複雑なルータトポロジを組みたいという要件はほとんど存在しない。OVNやNSXの柔軟性は、既存の物理ネットワーク設計をそのまま仮想化しようとした結果であり、ユーザの本質的な要求から導き出されたものではない。

---

## データモデル

### 階層構造

```
Tenant
 └── Network（隔離の単位、独立したIPアドレス空間）
      ├── Group（VMの集合）
      ├── Policy（Group間の通信許可）
      ├── Port（VMのネットワーク接続点）
      ├── Egress（外部向け接続）
      ├── Ingress（外部からの受け口）
      ├── Service Insertion（トラフィック経由点）
      └── Load Balancer（内部LB）
```

### DBスキーマ

```sql
CREATE TABLE tenants (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL
);

CREATE TABLE networks (
    id UUID PRIMARY KEY,
    tenant_id UUID REFERENCES tenants(id),
    name TEXT NOT NULL,
    cidr CIDR NOT NULL,
    vni INTEGER UNIQUE NOT NULL,
    UNIQUE(tenant_id, name)
);

CREATE TABLE groups (
    id UUID PRIMARY KEY,
    network_id UUID REFERENCES networks(id),
    name TEXT NOT NULL,
    UNIQUE(network_id, name)
);

CREATE TABLE ports (
    id UUID PRIMARY KEY,
    network_id UUID REFERENCES networks(id),
    group_id UUID REFERENCES groups(id),
    vm_id UUID NOT NULL,
    mac_address MACADDR NOT NULL,
    ip_address INET NOT NULL,
    host_id UUID NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    role TEXT NOT NULL DEFAULT 'default',
    UNIQUE(network_id, ip_address),
    UNIQUE(network_id, mac_address),
    UNIQUE(vm_id, role)
);

CREATE TABLE policies (
    id UUID PRIMARY KEY,
    network_id UUID REFERENCES networks(id),
    src_group_id UUID REFERENCES groups(id),
    dst_group_id UUID REFERENCES groups(id),
    protocol TEXT NOT NULL,
    dst_port INTEGER,
    priority INTEGER NOT NULL DEFAULT 1000,
    action TEXT NOT NULL DEFAULT 'allow',
    UNIQUE(network_id, src_group_id, dst_group_id, protocol, dst_port)
);

CREATE TABLE egresses (
    id UUID PRIMARY KEY,
    network_id UUID REFERENCES networks(id),
    type TEXT NOT NULL,
    config JSONB NOT NULL
);

CREATE TABLE ingresses (
    id UUID PRIMARY KEY,
    network_id UUID REFERENCES networks(id),
    type TEXT NOT NULL,
    public_ip INET NOT NULL,
    config JSONB NOT NULL
);

CREATE TABLE gateway_nodes (
    id UUID PRIMARY KEY,
    host_id UUID NOT NULL,
    external_ip INET NOT NULL,
    internal_ip INET NOT NULL,
    status TEXT NOT NULL DEFAULT 'active'
);
```

### 設計判断

- **1 VM = 1 Network**: VMは1つのNetworkにしか所属できない
- **1 VM = 1 Group**: VMは1つのGroupにのみ属する。複数Groupが必要に見えるケースはPolicyで表現する
- **デフォルト全拒否**: 同一Group内の通信も含めてデフォルトで拒否。明示的なPolicy定義が必要
- **Groupの粒度で制御**: タグやラベルのような第二の分類軸は持たない。細かい制御が必要なら、Groupを細かく分ける
- **Service Insertion用VMのみ2ポート可**: 通常VMは1ポート、Service Insertion用VMはservice_in/service_outの2ポートを持てる

---

## IPアドレス管理

### 基本方針

各VMを/30で隔離し、全通信をOVSのポリシーパイプラインに通す。

```
/30 = 4アドレス中2つ使用
  .0 ネットワークアドレス
  .1 VM
  .2 ゲートウェイ（OVS内の仮想ルータ、実体はフロー）
  .3 ブロードキャスト
```

VMから見ると普通のNICに普通のIPが振られている。ゲートウェイ（OVS）を必ず経由するため、すべての通信がOpenFlowパイプラインのポリシー評価を通る。

### CIDRプール

Network単位でCIDRプールを持つ。

- **デフォルト**: システムが自動割当（100.64.0.0/10レンジから）
- **VPN/専用線接続時**: CIDRの重複を避けるため、ユーザが明示指定必須
- **同一テナント内**: システムが重複しないよう自動管理

### IP採番

NetworkのCIDRから/30ブロックを順番に払い出す。削除されたVMのIPは再利用しない（conntrackのステートが残るリスク回避）。枯渇したら圧縮を検討。

### DHCPで配布する情報

```
IP:      100.64.0.1
Mask:    255.255.255.252 (/30)
Gateway: 100.64.0.2
DNS:     100.64.0.2（ゲートウェイと同一、エージェントが応答）
```

---

## アーキテクチャ

### 全体構成

```
         Internet
            ↑↓
       Border Router (ECMP)
         ↑↓    ↑↓
       GW-1    GW-2   ... (外部+内部に足を持つゲートウェイノード)
         ↑↓    ↑↓
    内部ファブリック (L3 Clos, Geneve)
      ↑↓  ↑↓  ↑↓
    Host-A Host-B Host-C (内部のみ)
```

### アンダーレイ

Geneveトンネルはホスト間でIP到達性があれば張れるため、アンダーレイはシンプルなL3ファブリックで十分。EVPN/VXLANは不要。

- L3 Closトポロジ（Spine-Leaf）
- BGPでルート交換（eBGP unnumberedが最もシンプル）
- MTU 9000以上（Geneveヘッダ分の余裕）
- ECMPでマルチパス

### 各ホストのコンポーネント

```
各ワーカーホスト:
  OVS (br-int)     - データプレーン
  cirrus-agent     - OVS制御 + DNS + メタデータサービス + DHCP応答
  libvirt/QEMU     - VM管理
```

### コントローラとエージェントの役割分担

**コントローラ（controllerロール）**:
- データモデル（Network, Group, Policy）の管理
- IPアドレス採番
- 各ホストのエージェントに「あるべき状態」を配信
- HostNetworkStateの計算と配信
- GWノードの割り当てとシャーディング

**エージェント（workerロール）**:
- OVSのOpenFlowフロー管理
- DHCP応答の生成
- DNS応答の生成
- メタデータサービスの提供
- ローカルVMのヘルスチェック

### コントローラとエージェントの通信

gRPCで通信する。既存のCirrusのcontroller/worker gRPC設計と一致。

**push型**: 状態変更時にコントローラがエージェントに通知

**pull型**: エージェント起動時にコントローラから自ホストの全状態を取得（リカバリ用）

gRPCのserver streamingで、初回に全状態を送り、以降は差分をストリーミング。

```protobuf
message HostNetworkState {
  repeated PortState ports = 1;
  repeated PolicyRule policies = 2;
  repeated RemotePort remote_ports = 3;
  repeated EgressRule egresses = 4;
  repeated DnsRecord dns_records = 5;
}

message PortState {
  string port_id = 1;
  string vm_id = 2;
  string network_id = 3;
  string group_id = 4;
  string mac_address = 5;
  string ip_address = 6;
  string gateway_ip = 7;
}

message RemotePort {
  string network_id = 1;
  string group_id = 2;
  string ip_address = 3;
  string host_ip = 4;
}

message DnsRecord {
  string name = 1;
  string ip = 2;
  string network_id = 3;
}
```

エージェントは宣言的な状態を受け取り、現在のOVSフローとの差分を計算してOpenFlowを更新する。

---

## データプレーン

### OVSの役割

OVS（Open vSwitch）はプログラマブルなソフトウェアスイッチで、OpenFlowプロトコルでフローテーブルを制御する。Cirrusではデータプレーンとしてのみ使用し、OVNは使用しない。

### OpenFlowパイプライン概要

パイプラインの詳細設計は実装フェーズで確定するが、概念的な構成は以下の通り。

```
Table 0: 入力分類 + port security
Table 1: conntrack（確立済みセッションのバイパス）
Table 2: 宛先GROUP_ID解決
Table 3: Policy評価（src_group × dst_group × protocol × port）
Table 4: 宛先ホスト解決 + ローカル/リモート判定
Table 5: Geneveカプセル化（リモートの場合）
Table 6: ローカル出力
Table 7: Egress処理
```

スケール上重要な設計判断として、宛先IPからGROUP_IDを先に解決してregisterにセットし、Policy評価はGroup対Groupのマッチで行う。これによりフロー数がVM数ではなくGroup数に比例し、スケール特性が大幅に改善する。

### アクセス制御

AWSのSecurity Groupに相当する機能をOVSのconntrackで実現する。

**ステートフル制御（Policy）**: conntrackで新規接続をPolicy評価し、確立済みセッションは自動許可。戻りの通信は`ct_state=+est`で自動的にマッチ。

**deny/allowの優先度制御**: priorityフィールドで「tcp/443は拒否、それ以外は全許可」のような表現も可能。

---

## ゲートウェイノード

### 必要性

物理ホストはプライベートIPしか持たず、インターネットに直接露出していない。そのため、インターネットとの境界にGateway Nodeが必要。

```
GWノード:
  eth0: 外部セグメント（パブリックIP到達性あり）
  eth1: 内部ファブリック（Geneveアンダーレイ）
```

### スケールアウト

GWノードは横にスケールアウトできる。Network単位でGWノードのペアを割り当てる。

```
GW-1 (Act) ←→ GW-2 (Stby) : Network-A, Network-B
GW-3 (Act) ←→ GW-4 (Stby) : Network-C, Network-D
GW-5 (Act) ←→ GW-6 (Stby) : Network-E, Network-F
```

Network数が増えたらGWペアを追加。各ホストのエージェントは「このNetworkの外部通信はGW-1に送る」とフローに持つ。

### HA構成

Active-Standbyで構成し、BFDで死活監視。障害時はStandbyに切り替え。

conntrack同期は初期実装では行わず、GW切り替え時に既存セッションが切断されることを許容する。

### GWノードの無停止移動

NetworkのGWを別ノードに移動する際は、ライブマイグレーションと同じFallbackパターンを使用。

1. 新GWにフローを準備、外部IPを付与
2. 各ホストのフローを新GW向けに切り替え（ACK待ち）
3. 旧GWに既存セッションのdrainフローを設定（ct_state=+estのみ処理、+newは新GWに転送）
4. 既存セッションの自然タイムアウトを待つ
5. drain完了後、旧GWからフロー削除

GWノード群を同一外部セグメントに配置し、Ingress側の切り替えはGratuitous ARPで完結させる。

---

## 外部接続（Egress）

### NAT Gateway（Internet接続）

VMからインターネットへの通信。GWノードでSNATする。

```
VM → OVS → Geneve → GWノード → SNAT → Internet
```

パブリックIPの共有はホストごとにポート範囲を分割して衝突を回避。初期実装ではNetworkあたり1パブリックIP。

### VPN接続

テナントのNetworkとオンプレ拠点をIPsecやWireGuardで接続。GWノードに集約。

```
VM → OVS → Geneve → GWノード → IPsec/WireGuard → オンプレ
```

VPN接続するNetworkはCIDRのユーザ指定が必須。

### Direct Connect（専用線）

専用線はBorder Routerに接続し、GWノードへはVLANで届ける。GWノードのNICを増やす必要はない。

```
オンプレ ── 専用線 ── Border Router ── VLAN trunk ── GWノード
```

```
GWノード eth0 (trunk):
  VLAN 10:  Internet（パブリックIP）
  VLAN 100: 専用線・顧客A
  VLAN 200: 専用線・顧客B
```

BGPの管理はBorder Router側で行い、Cirrusには持ち込まない。Cirrusの管理範囲とネットワーク機器の管理範囲を明確に分離する。

### Service Endpoint（テナント間サービス公開）

テナント間のL3 Peeringは提供しない。代わりにService Endpoint方式で限定的なサービス公開を行う。

提供側テナントがサービスを公開し、消費側テナントからDNS名でアクセスする。境界で双方向NAT（DNAT + SNAT）を行い、テナント間のIPアドレス空間を完全に隔離したまま通信する。CIDRの重複問題が根本的に消える。

Service Endpoint用のCIDRプール（100.127.0.0/16）をシステム全体で確保し、VIPを割り当てる。

---

## 外部からの受け口（Ingress）

### Direct IP

パブリックIPを特定のVMまたはL4 Load Balancerに1対1で紐づける。GWノードでDNATを実行。

```
VM宛:   Internet → GWノード → DNAT → Geneve → VMのホスト
LB宛:   Internet → GWノード → DNAT → LBのDNATフロー → Geneve → VMホスト
```

Direct IPのターゲットにL4 LBを指定できることで、「固定IPでWebサービスを公開し、裏側はLBで分散」という典型的なユースケースに対応する。AWSのNLBにElastic IPを割り当てる構成と同等。

### グローバルFQDNの割り当て

パブリックIPに対するグローバルFQDN（例: `api.example.com → 203.0.113.10`）の管理はCirrusの範囲外。Cirrusが返すのはパブリックIPのみで、それをどのFQDNに紐づけるかはテナントが自身のDNSプロバイダ（Route 53、Cloudflare等）で設定する。将来PaaS層でマネージドDNSサービスを構築する場合は、IngressのパブリックIPにFQDNを自動割り当てする機能をPaaSレイヤーで提供する。

### L4 Load Balancer

パブリックIPでトラフィックを受け、Group内の複数VMにOVSのconntrack + DNATで分散する。GWノード上で実行。

```
Internet → GWノード → DNAT(ラウンドロビン) → Geneve → VMホスト
```

conntrackがセッションアフィニティを保証する。

ヘルスチェックはコントローラ主導で行う。各ホストのエージェントがローカルVMにヘルスチェックを実行し、結果をコントローラに報告。コントローラがGWノードのエージェントに生存VM一覧を通知し、DNATフローを更新する。

### 内部LB

テナント内のVM間ロードバランシング。GroupにVIPを割り当て、各ホストのOVSで分散実行する。GWノードを経由しない。

DNSで`api-lb.my-app.internal`を解決するとVIPが返り、OVSがGroup内のVMにDNATで分散する。

---

## Service Insertion

L7 LB、FW、IPS等のアプライアンスをトラフィック経路に挿入するための汎用プリミティブ。Cirrusはアプライアンスの種類や設定内容を関知しない。

テナントが任意のVMイメージをデプロイし、Service Insertionで経路に挿入する。

```
通常:      web VM → OVS → api VM
挿入後:    web VM → OVS → FW VM(in) → 検査 → FW VM(out) → OVS → api VM
```

Service Insertion用VMはservice_in / service_outの2ポートを持つ。

### IaaS / PaaS の分離

```
IaaS（Cirrus）:
  Network, Group, Policy, Ingress, Egress, Service Insertion
  → テナントが自由にVMを配置して好きな構成を作る

PaaS（将来、Cirrusの上に構築）:
  マネージドLB   → Service Insertion + nginx VMを自動デプロイ
  マネージドFW   → Service Insertion + FW VMを自動デプロイ
  マネージドDB   → VM + Policy自動設定
  → IaaSのプリミティブを組み合わせて自動化するレイヤー
```

PaaSがIaaSのAPIだけを使って構築できることが、IaaSの抽象化の正しさの証明になる。

---

## ライブマイグレーション

### フロー更新手順

VMがHost-AからHost-Bに移動する際、更新が必要な箇所は3つ。

1. **Host-B（移動先）**: フロー新規作成
2. **Host-A（移動元）**: フロー削除
3. **他の全関連ホスト**: Geneveトンネルの宛先更新

### シーケンス

```
コントローラ                Host-A agent      Host-B agent      他ホスト agent
    │                          │                  │                  │
    │── 事前準備 ─────────────────────────────────→│                  │
    │                          │          ポート+フロー準備          │
    │                          │                  │                  │
    │   (Phase 1: メモリコピー)│                  │                  │
    │                          │                  │                  │
    │   (Phase 2: VM一時停止)  │                  │                  │
    │                          │                  │                  │
    │── フロー有効化 ─────────────────────────────→│                  │
    │── Fallback転送設定 ────→│                  │                  │
    │── DB更新 (host_id)       │                  │                  │
    │                          │                  │                  │
    │   (Phase 3: VM再開)      │                  │                  │
    │                          │                  │                  │
    │── トンネル宛先更新 ────────────────────────────────────────────→│
    │                          │                  │              ACK返送
    │                          │                  │                  │
    │── 全ACK受信              │                  │                  │
    │── Fallback削除 ────────→│                  │                  │
    │                     フロー削除               │                  │
```

### Fallback転送

他ホストのフロー更新が完了する前にVMが再開しても、Host-AがFallbackとしてHost-Bに転送するためパケットロスがゼロ。

### ACK管理

各ホストのエージェントはフロー更新完了後にACKを返送。コントローラは全ACK受信後にFallbackを削除する。

タイムアウト（30秒）後の未応答ホストはログに記録してFallback削除。未応答ホストは次のエージェント再接続時に全状態を再同期して修正される。

### portsテーブルの状態遷移

```
active → migrating → switching → draining → active
         Phase 1開始  フロー切替中  Fallback残存  全ACK受信
                                    ACK待ち      Fallback削除完了
```

### 冪等性

コントローラ再起動時にstatusがdrainingのポートがあれば、未応答ホストの確認とFallback削除を再開できる。

---

## DNS

### 設計方針

IPアドレスを隠蔽するモデルにおいて、DNSはVM間通信の必須インフラ。エージェント内にDNSサーバを組み込み、CoreDNS等の外部依存を持たない。

### レコードの種類

```
# VM個別
vm-1.api.my-app.internal → 100.64.0.1

# Group全体（Aレコード複数返し）
api.my-app.internal → 100.64.0.1, 100.64.0.5

# 内部LBのVIP
api-lb.my-app.internal → 100.64.255.1

# Service Endpoint
my-api.tenant-b.service.internal → 100.127.0.1

# 逆引き
100.64.0.1 → vm-1.api.my-app.internal
```

### レコード管理

コントローラからのHostNetworkState配信にDNSレコードを含める。エージェントは既にポート情報を持っているので、追加のデータ配信は不要。

### Network間の隔離

エージェントはDNS問い合わせの送信元IPからNetwork IDを解決し、そのNetworkに属するレコードだけを返す。異なるNetworkのレコードは引けない。

### 外部DNSフォワード

内部レコードに該当しない問い合わせ（google.com等）は、エージェントが外部DNSリゾルバにフォワードする。エージェントのプロセスはホストのネットワークスタック上で動作するため、ホストのデフォルトルートで外部に出られる。

---

## メタデータサービス

### 概要

AWSの169.254.169.254に相当する機能。VMが自身のID、Network、Group、IP等をHTTPで取得できる。エージェント内に組み込みHTTPサーバとして実装。

### パケットの流れ

```
VM → OVS → (169.254.169.254宛をエージェントに転送) → エージェント内HTTPサーバ
```

送信元IPからVMを識別し、そのVMのメタデータを返す。

### レスポンス

```json
GET /latest/meta-data/
{
  "vm_id": "vm-uuid",
  "vm_name": "api-1",
  "network": {
    "id": "net-uuid",
    "name": "my-app"
  },
  "group": {
    "id": "group-uuid",
    "name": "api"
  },
  "interfaces": [
    {
      "ip": "100.64.0.1",
      "gateway": "100.64.0.2",
      "subnet_mask": "255.255.255.252",
      "mac": "fa:16:3e:aa:bb:cc",
      "dns": "100.64.0.2"
    }
  ],
  "hostname": "api-1.api.my-app.internal",
  "tenant_id": "tenant-uuid"
}
```

### cloud-initとの統合

```
VM起動:
  1. DHCP: IP, GW, DNS, maskを取得
  2. ネットワーク接続確立
  3. メタデータサービスからホスト名、ユーザデータ取得
  4. cloud-init完了
```

ネットワーク設定はDHCPで配布するため、NoCloudでの事前注入は不要。

---

## API設計

既存のCirrusの方針（async 202 Accepted + ステータスポーリング、X-API-Key認証）に従う。

### エンドポイント一覧

```
テナント向け:
  /tenants/{tid}/networks                           - Network CRUD
  /tenants/{tid}/networks/{nid}/groups              - Group CRUD
  /tenants/{tid}/networks/{nid}/policies            - Policy CRUD
  /tenants/{tid}/networks/{nid}/egresses            - Egress CRUD
  /tenants/{tid}/networks/{nid}/ingresses           - Ingress CRUD
  /tenants/{tid}/networks/{nid}/service-insertions  - Service Insertion CRUD
  /tenants/{tid}/networks/{nid}/load-balancers      - 内部LB CRUD
  /tenants/{tid}/networks/{nid}/service-connections - Service Endpoint消費側
  /tenants/{tid}/networks/{nid}/ports               - Port GET のみ公開

内部API（Computeモジュールから呼び出し）:
  /tenants/{tid}/networks/{nid}/ports               - Port CRUD

管理者向け:
  /gateway-nodes                                     - GWノード管理
  /ip-pools                                          - パブリックIPプール管理
```

### Network

```json
// POST /tenants/{tid}/networks
// 自動CIDR割当
{ "name": "my-app" }

// VPN/専用線用（CIDR指定）
{ "name": "hybrid-app", "cidr": "10.200.0.0/16" }

// Response: 202 Accepted
{
  "id": "net-uuid",
  "name": "my-app",
  "cidr": "100.64.0.0/16",
  "vni": 1001,
  "status": "creating"
}
```

### Group

フロー変更が発生しないため同期レスポンス。

```json
// POST /tenants/{tid}/networks/{nid}/groups
{ "name": "api" }

// Response: 201 Created
{ "id": "group-uuid", "name": "api", "network_id": "net-uuid" }
```

### Policy

```json
// POST /tenants/{tid}/networks/{nid}/policies
{
  "src_group": "api",
  "dst_group": "db",
  "protocol": "tcp",
  "dst_port": 5432,
  "priority": 1000,
  "action": "allow"
}

// 同一Group内の全許可
{
  "src_group": "web",
  "dst_group": "web",
  "protocol": "any",
  "priority": 1000,
  "action": "allow"
}

// Response: 202 Accepted
```

### Port（内部API）

テナントはVM作成時にNetworkとGroupを指定するだけ。Port作成はCirrusが内部的に行う。

```json
// テナントが呼ぶVM作成API
POST /tenants/{tid}/vms
{
  "name": "api-1",
  "image": "ubuntu-24.04",
  "flavor": "m1.small",
  "network": "my-app",
  "group": "api"
}

// Cirrus内部:
// 1. Compute: VMリソース確保、ホスト選定
// 2. Network: Port作成（内部API）
// 3. Compute: cloud-initにネットワーク情報を注入
// 4. Compute: VM起動
```

### Egress

```json
// NAT Gateway
{ "type": "nat", "name": "internet-gw" }

// VPN
{
  "type": "vpn",
  "name": "office-vpn",
  "config": {
    "protocol": "wireguard",
    "remote_endpoint": "1.2.3.4",
    "remote_cidr": "192.168.0.0/16",
    "shared_key": "..."
  }
}

// Direct Connect
{
  "type": "direct_connect",
  "name": "dc-link-1",
  "config": { "remote_cidr": "192.168.0.0/16", "vlan_id": 100 }
}

// Service Endpoint（提供側）
{
  "type": "service_endpoint",
  "name": "my-api",
  "config": {
    "group": "api",
    "port": "tcp/443",
    "allowed_tenants": ["tenant-a-uuid"]
  }
}
```

### Ingress

```json
// Direct IP（VM宛）
{
  "type": "direct_ip",
  "name": "bastion-ip",
  "config": { "target_vm": "vm-uuid" }
}

// Direct IP（LB宛）
{
  "type": "direct_ip",
  "name": "web-fixed-ip",
  "config": { "target_load_balancer": "lb-uuid" }
}

// L4 Load Balancer
{
  "type": "load_balancer",
  "name": "web-lb",
  "config": {
    "target_group": "web",
    "port": "tcp/443",
    "algorithm": "round_robin",
    "health_check": { "protocol": "tcp", "port": 443, "interval_seconds": 10 }
  }
}
```

### Service Insertion

```json
// POST /tenants/{tid}/networks/{nid}/service-insertions
{
  "name": "web-firewall",
  "target_group": "appliance-fw",
  "intercept": [
    {
      "src_group": "web",
      "dst_group": "api",
      "protocol": "tcp",
      "dst_port": 8080
    }
  ]
}
```

### Service Connection（消費側）

```json
// POST /tenants/{tid}/networks/{nid}/service-connections
{ "endpoint_id": "提供側のservice_endpoint UUID" }

// Response: 202 Accepted
{
  "id": "sc-uuid",
  "endpoint_id": "...",
  "dns_name": "my-api.tenant-b.service.internal",
  "vip": "100.127.0.1",
  "status": "connecting"
}
```

---

## Cirrusモジュール構成

ネットワーク機能はcirrusバイナリの内部モジュールとして実装する。

```
internal/network/
  ├── model.go       (Network, Group, Policy)
  ├── ipam.go        (IPアドレス採番)
  ├── controller.go  (状態計算、エージェントへの配信)
  └── agent.go       (OVS制御、OpenFlow変換、DNS、DHCP、メタデータ)
```

controller.goはcontrollerロールで動作し、agent.goはworkerロールで動作する。

---

## スケーラビリティ

### OVNとの比較

OVNのボトルネックはovn-northd（シングルスレッド、全論理フロー再計算）とOVSDB（全ホスト接続）。1,000ホスト/10万ポート程度が現実的な上限。

### VPCモデルの利点

- **シャーディング**: Network間が独立しているため、コントローラをNetwork/テナント単位で分割できる
- **差分更新**: 「このNetworkのこのGroupにVMが追加された」→ 関係するホストにだけフロー追加。影響範囲が論理的に閉じている
- **フロー数の予測性**: 1つのVPCあたりのフロー数はGroup数とPolicy数に比例。ホスト単位では自ホスト上のVMが存在するNetworkのフローだけ保持

### GWノードのスケール

Network数に応じてGWペアを追加。特定Networkの外部トラフィックが異常に多い場合は、該当Networkを別GWペアに移動。

---

## 将来の拡張（初期実装には含めない）

- **Group Alias**: 複数Groupの集合を定義し、Policyの記述をDRYにする
- **CIDR指定のPolicy**: 外部CIDRレンジからのアクセス制御（VPN/専用線経由のトラフィック向け）
- **QoS/帯域制御**: ノイジーネイバー問題の対処
- **テナント向け可観測性**: トラフィック量やPolicy hit数の可視化
- **conntrack同期**: GWノードのHA切り替え時の既存セッション維持
- **BGP統合によるDirect IPの分散Ingress**
