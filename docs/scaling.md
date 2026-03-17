# スケーリング課題と拡張パス

## 段階的な拡張ロードマップ

| 規模 | 対応 |
|---|---|
| ~10台 | 現設計のまま |
| ~50台 | スケジューラにインメモリキャッシュ追加、heartbeatをRedisに分離 |
| ~100台 | BGP EVPN導入（FRRouting）、controller HA（Active-Active） |
| ~500台 | セル分割、イメージ配布にP2P or 共有ストレージ、NATS等で非同期化 |

## 致命的な問題（規模拡大時）

### 1. VXLANフルメッシュの破綻

worker数Nに対してトンネル数がN×(N-1)/2。500台で約125,000トンネル。OVSのフロー数も爆発する。

**解決策**: BGP EVPN + VXLANへの移行。各workerがBGPスピーカーになり、ルートリフレクタ経由で必要なトンネルだけ動的に張る。現設計のcontrollerがフルメッシュを指示する方式はせいぜい数十台が限界。

### 2. スケジューラの全workerスキャン

VM作成のたびに全workerのリソース集計が走る。

**解決策**: workersテーブルにused_vcpus/used_ram_mbのキャッシュカラム、またはスケジューラ内にインメモリのリソースマップ。同時VM作成時の楽観ロック/競合制御も必要。

### 3. controller→worker gRPCボトルネック

500台へのgRPC接続数とgoroutine数が問題になる。

**解決策**: セル分割（50台ずつのセルにsub-controller）か、非同期メッセージキュー（NATS、Redis Streams）で疎結合化。

## 重大だが段階的に対処可能

### 4. PostgreSQL単体の限界

VM数が数万台、ポート数が数十万でもインデックスが適切なら捌ける。問題はheartbeatの書き込み頻度（500台×10秒ごと=秒間50 writes）によるVACUUM負荷。

**解決策**: heartbeatをRedisやetcdに分離。

### 5. イメージ配布

500台のworkerにイメージコピーは現実的でない。

**解決策**: NFS/Cephの共有ストレージ、BitTorrent的P2P配布、またはレジストリからworkerがpull。image.Storeインターフェースはこの拡張に対応可能。

### 6. Controller HA

controller単一障害点。既存VMは動き続けるがAPI/スケジューリングが停止。

**解決策**: Active-Active（ステートレスAPIサーバ複数台 + 共有DB）。現設計はステートをDBに集約しているため比較的やりやすい。

## 現設計が耐えるところ

- **モジュラーモノリス**: マイクロサービス分割では上記の問題は解決しない。分散トランザクションの問題が増える。セル分割時もセル内はモノリスのままでよい。
- **DBスキーマ**: driver_dataを排除したクリーンな設計。パーティショニング（project_id, worker_id）でスケール可能。
- **interface設計**: network.ProviderがBGP EVPN対応を受け入れられる構造になっている。

## 開発環境

### ネステッドKVM構成

| VM | vCPU | RAM | Disk | 用途 |
|---|---|---|---|---|
| controller | 2 | 2GB | 20GB | API + PostgreSQL + スケジューラ |
| worker-01 | 4 | 8GB | 60GB | libvirt + OVS + ゲストVM |
| worker-02 | 4 | 8GB | 60GB | 同上 |
| **合計** | **10** | **18GB** | **140GB** | |

### 最小構成（controller相乗り）

controllerをホスト上で直接実行し、workerのみVM化。ホスト8GB RAMでも動作可能。

### ネステッドKVM有効化

```bash
# Intel
echo "options kvm_intel nested=1" > /etc/modprobe.d/kvm.conf
# AMD
echo "options kvm_amd nested=1" > /etc/modprobe.d/kvm.conf
# 確認
cat /sys/module/kvm_intel/parameters/nested  # Y
```

worker VMのCPUモードは `host-passthrough` が必須（ゲストVM内でKVMを使用するため）。
