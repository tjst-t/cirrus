# マルチテナンシーと認可

## テナントモデル

**組織 (Organization)** — 契約・課金の最上位単位。企業や部門に対応。

**テナント (Tenant/Project)** — リソースの所有・クォータの単位。組織の中に複数存在（dev, staging, prod等）。

**ユーザ** — 認証の単位。組織に所属し、複数のテナントに対して異なるロールを持てる。

```
Organization (ACME Corp)
├── Tenant: dev
│   ├── User-A (tenant_admin)
│   └── User-B (tenant_member)
├── Tenant: staging
│   ├── User-A (tenant_member)
│   └── User-C (tenant_admin)
└── Tenant: prod
    └── User-A (tenant_admin)
```

## 隔離

### セキュリティ境界（絶対要件）

他テナントのVM、ボリューム、データ、ネットワークトラフィックが見えない。

### 性能境界（ベストエフォート）

バックエンドのQoS機能でボリューム単位のIOPS/帯域制限。ボリュームタイプにQoSポリシーを紐づけ。完全な性能隔離は共有型では難しく、SLAとして明確に定義。

### リソースクォータ

テナントごとのリソース上限。組織全体に上限を設けつつ、テナントごとに配分する階層化クォータ。

## クォータ

全リソースタイプに対してテナント単位で設定。

- vCPU数
- メモリ量
- ボリューム容量・数
- スナップショット数
- ネットワーク数
- フローティングIP数

基本はハードクォータだが、既存ボリュームの動的な容量増加は超過を許容してアラート（書き込み停止はVMを壊すリスク）。新規リソース作成はブロック。

```
Organization: ACME Corp (total: 500 vCPU, 2TB RAM)
├── Tenant: dev      (quota: 100 vCPU, 400GB RAM)
├── Tenant: staging  (quota: 100 vCPU, 400GB RAM)
└── Tenant: prod     (quota: 300 vCPU, 1.2TB RAM)
```

## リソースの所属

| 分類 | リソース |
|------|----------|
| テナントに属さない（インフラリソース） | ホスト、バックエンド、ネットワークドメイン |
| テナントに属する | VM、ボリューム、仮想ネットワーク、ポート、ルータ、セキュリティグループ、テンプレート（テナント作成分） |
| テナントに属さないがアクセス制御あり | ボリュームタイプ、Flavor（テナントごとに利用可能なものを制限可能） |

## 認可モデル

### ロール（固定RBAC）

| ロール | スコープ | 権限 |
|--------|----------|------|
| インフラ管理者 | テナント横断 | ホスト、バックエンド、ネットワークドメインの管理 |
| 組織管理者 | 組織 | テナントの作成・削除、クォータ配分 |
| テナント管理者 | テナント | テナント内の全操作、テナント内ユーザへのロール付与 |
| テナントメンバー | テナント | テナント内の日常操作。破壊的操作は制限 |

### ロール割り当て

`(user_id, scope_type, scope_id, role)` の三つ組。scope_typeがorganizationならorg_id、tenantならtenant_idがscope_id。

### 認可インターフェース

全ての認可判定は `authorize(user, action, resource) -> allow/deny` を通る。初期はロールチェックのみだが、将来ポリシーエンジン（OPA等）への差し替えが呼び出し側の変更なしに可能な設計。resourceを最初から渡すことで、リソース属性に基づく判定への拡張も可能。

```go
// internal/auth/authorizer.go
package auth

type Authorizer interface {
    Authorize(ctx context.Context, user User, action Action, resource Resource) (Decision, error)
}

type Decision int

const (
    Allow Decision = iota
    Deny
)
```

## 認証

外部IdP（Keycloak/Okta等）とOIDC連携。開発初期は静的設定ファイルかAPIトークン。
