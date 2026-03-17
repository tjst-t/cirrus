# Cirrus Design Documents

Cirrus はセルフサービス型プライベートIaaSプラットフォームです。
OpenStack のようなAPI駆動のVM管理を、単一バイナリのモジュラーモノリスとして実現します。

## ドキュメント一覧

| ドキュメント | 内容 |
|---|---|
| [architecture.md](./architecture.md) | 全体アーキテクチャ、コンポーネント構成、ディレクトリ構成 |
| [database.md](./database.md) | DBスキーマ、ER図、設計判断 |
| [api.md](./api.md) | REST APIエンドポイント、認証、リクエスト/レスポンス |
| [compute.md](./compute.md) | libvirtによるVM管理、domain XML、cloud-init |
| [network.md](./network.md) | OVS + VXLANによるテナントネットワーク分離 |
| [storage.md](./storage.md) | ストレージ抽象化、バックエンド設計 |
| [sequences.md](./sequences.md) | VM作成までの全体フロー（シーケンス図） |
| [scaling.md](./scaling.md) | スケーリング課題と段階的拡張パス |

## 設計方針

- **単一バイナリ**: `cirrus controller` / `cirrus worker` でロール切り替え
- **モジュラーモノリス**: Goのinterfaceでcompute/network/storageを抽象化し、バックエンド差し替え可能
- **APIファースト**: テナントがAPIでリソースを要求し、プラットフォームが配置を決定
- **非同期操作**: VM作成は202 Accepted → ステータスpolling
- **YAGNI**: driver_data JSONBはnullableで、必要なバックエンドだけ使用

## Phase 1 構成

- 1 Controller + 2 Worker（最小は1 Worker）
- 開発環境はネステッドKVM（controller/workerをVMとして構築）
- ストレージ: ローカルqcow2
- ネットワーク: OVS + VXLAN
