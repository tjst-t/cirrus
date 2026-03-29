# 開発・テスト方針

## 基本方針

シミュレータ群をcirrusリポジトリに統合。3レイヤーのテスト戦略を採用する。

シミュレータは本番と同じプロトコルを話すため、Cirrus側のコードにテスト用の分岐やモックは不要。接続先の設定を切り替えるだけでシミュレータ環境と本番環境を行き来できる。

## テスト戦略

### レイヤー1: ビジネスロジック（Goユニットテスト）

OVSもネットワークも不要。純粋なGoのユニットテストで完結する。テスト対象の大半がここ。

- IPAM（/30採番、CIDR管理）
- Policy評価ロジック
- HostNetworkState計算
- DNSレコード生成
- メタデータレスポンス生成
- スケジューラのcapability-basedマッチング
- クォータの階層化チェック
- 認可判定ロジック

### レイヤー2: OVSフロー変換（モッククライアント）

HostNetworkState→OpenFlowフローへの変換ロジックをテスト。MockOVSClient interfaceでコマンド記録・検証。

```go
type MockOVSClient interface {
    AddFlow(table int, priority int, match string, actions string) error
    DeleteFlow(table int, match string) error
    AddPort(bridge string, port string) error
    DeletePort(bridge string, port string) error
    GetRecordedCommands() []OVSCommand
}
```

### レイヤー3: 結合テスト（実OVS + docker-compose）

cirrus-sim-workerコンテナで実行。実際にパケットを流して検証する。

- コントローラ→エージェント gRPC通信
- HostNetworkState配信と差分更新
- OVSフロー注入（実OVS）
- VM(namespace)間のGeneveトンネル通信
- DHCP応答
- DNS応答とNetwork隔離
- メタデータサービス
- Policy（conntrack）による通信許可/拒否
- ライブマイグレーション（namespace移動 + フロー更新）

## シミュレータ構成（cirrusリポジトリ内）

| シミュレータ | プロトコル | 配置 |
|---|---|---|
| libvirtd-sim | libvirt RPC | test/sim/libvirtd/ |
| storage-sim | REST API | test/sim/storage/ |
| awx-sim | AWX REST API | test/sim/awx/ |
| OVSモック | Go interface | test/mock/ovs/ |

Note: OVN-simは廃止。OVSは結合テストで実物を使用。

## docker-compose構成

```yaml
services:
  controller:
    image: cirrus
    command: cirrus --role controller
    depends_on: [postgres]

  postgres:
    image: postgres:16

  worker-1:
    image: cirrus-sim-worker
    privileged: true
    networks: [fabric]

  worker-2:
    image: cirrus-sim-worker
    privileged: true
    networks: [fabric]

  worker-3:
    image: cirrus-sim-worker
    privileged: true
    networks: [fabric]

  storage-sim:
    image: cirrus-sim-storage

  awx-sim:
    image: cirrus-sim-awx

networks:
  fabric:
    driver: bridge
```

## cirrus-sim-workerイメージ

### Dockerfile

```dockerfile
FROM ubuntu:24.04

# OVS（実物）
RUN apt-get install -y openvswitch-switch

# cirrus-agent（実バイナリ）
COPY cirrus /usr/local/bin/cirrus

# libvirtd-sim（シミュレータ）
COPY libvirtd-sim /usr/local/bin/libvirtd-sim

COPY entrypoint.sh /entrypoint.sh
```

### entrypoint.sh

```bash
#!/bin/bash
ovs-vswitchd &
ovsdb-server &
ovs-vsctl add-br br-int

libvirtd-sim &
cirrus --role worker &

wait
```

## VMシミュレーション

network namespaceとvethペアでVM代替。namespace内からping/curl/dig可能。

```bash
# VM作成時
ip netns add vm-${uuid}
ip link add vm-${uuid}-tap type veth peer name eth0 netns vm-${uuid}
ip link set vm-${uuid}-tap up
ovs-vsctl add-port br-int vm-${uuid}-tap
ip netns exec vm-${uuid} dhclient eth0
```

Linuxのnetwork namespaceはネストではなくカーネル上の平坦な構造のため、Dockerコンテナ内（`--privileged`）でも問題なく動作する。

## 開発ワークフロー

### ローカル開発（レイヤー1/2）

```bash
make test
```

外部依存なし。純粋なGoテストで完結する。

### 結合テスト（レイヤー3）

```bash
docker-compose up
make test-integration
```

### CI

- レイヤー1/2: 通常のGoテスト
- レイヤー3: docker-compose + privileged containers
