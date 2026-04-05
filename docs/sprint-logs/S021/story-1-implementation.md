# S021-1: E2E テストスイート — 実装ログ

## 作成したファイル一覧

| ファイル | タスク |
|---|---|
| `test/integration/e2e_fullflow_test.go` | S021-1-1: テナント作成→VM削除フルフロー |
| `test/integration/e2e_multitenant_test.go` | S021-1-2: マルチテナントシナリオ（隔離確認） |
| `test/integration/e2e_policy_test.go` | S021-1-3: Policy/Group アクセス制御確認 |
| `test/integration/e2e_medium_env_test.go` | S021-1-4: medium 環境スモークテスト |

既存ファイルの修正:
| ファイル | 変更内容 |
|---|---|
| `test/integration/testutil.go` | `network.NewStore` の呼び出しに `nil` quota.Service 引数を追加（コンパイルエラー修正） |

## 各テストの概要

### `TestE2EFullFlow` (`e2e_fullflow_test.go`)
- Organization 作成 → Tenant 作成 → Network 作成 → Flavor 取得 → VM 作成（Network 接続）
- VM が `running` になるまで待機（最大 60 秒）
- VM の NetworkID が作成した Network と一致することを確認
- VM stop → VM delete → Network delete の順でリソースを削除
- 全リソースを `t.Cleanup` で確実にクリーンアップ
- `CIRRUS_ADMIN_TOKEN`（なければ `CIRRUS_TOKEN`）を管理者操作に使用

### `TestE2EMultiTenant` (`e2e_multitenant_test.go`)
- 同一 Organization 内に Tenant A / Tenant B を作成
- 各テナントに別の Network と VM を作成
- `cross_tenant_GET_network_forbidden`: Tenant A の ListNetworks に Tenant B の Network が含まれないことを確認
- `cross_tenant_DELETE_vm_forbidden`: Tenant A のスコープで Tenant B の VM を削除しようとすると 403/404 が返ることを確認
- `cross_tenant_DELETE_network_forbidden`: Tenant A のスコープで Tenant B の Network を削除しようとすると 403/404 が返ることを確認
- `tenant_A_sees_own_resources` / `tenant_B_sees_own_resources`: 各テナントが自身のリソースを参照できることを確認

### `TestE2EPolicyGroupAccessControl` (`e2e_policy_test.go`)
- DB 直接アクセスによるテスト（`TestEnv` 使用）
- Network を作成し、`allow-group` / `deny-group` の 2 グループを追加
- Port を `allow-group` に所属させ、GroupID が正しいことを確認
- TCP port 22 許可ポリシーを作成し DB に正しく保存されることを確認
- TCP port 80 拒否ポリシーも追加し、ポリシー合計が 2 件以上あることを確認
- allow-ssh ポリシーを削除後、DB から消えていることを確認
- `network.StateController.ComputeHostNetworkState` で削除したポリシーが host state に含まれないことを確認

### `TestE2EMediumEnvSmoke` (`e2e_medium_env_test.go`)
- `CIRRUS_SIM_ENV` が空の場合はスキップ
- `host_count_100_plus`: 登録ホスト数が 100 以上であることを確認（medium 環境は tokyo 250 + osaka 150 = 計 400 ホスト）
- `tenant_count_20_plus`: 全 Organization のテナント合計が 20 以上であることを確認（medium 環境は 20 テナントをプリロード）
- `vm_start_within_60s`: VM 作成から `running` 状態到達まで 60 秒以内であることを確認

## ビルド結果

```
$ go build -tags integration ./test/integration/...
# (exit 0 — エラーなし)

$ go vet -tags integration ./test/integration/...
# (exit 0 — エラーなし)
```

## 気づいた点・決定した事項

1. **既存 `testutil.go` のコンパイルエラー**: `network.NewStore` のシグネチャが `(pool, logger, quotaSvc)` に変更されていたが、`testutil.go` では 2 引数で呼び出していた。Network Store はすでに `quotaSvc == nil` をガードしているため、`nil` を渡すことで修正した。

2. **テナント/Organization の DELETE API 不在**: 現在の REST API にテナント・Organization の削除エンドポイントは存在しない。フルフローテストでは `t.Cleanup` でリソース（VM・Network）のみを削除し、テナント/Organization の削除はスキップする形とした。

3. **403 テストの実装方法**: `client.Client` は HTTP ステータスコードをエラー文字列 `"API error (403)"` としてラップする。マルチテナントテストでは `strings.Contains(err.Error(), "403")` でチェックした。所有権チェックにより 404 が返る可能性もあるため、403 または 404 の両方を正しい隔離とみなした。

4. **`pb.PolicyRule` の ID フィールド**: proto 生成コードでは `PolicyRule.ID` ではなく `PolicyRule.PolicyId` (string) を使うため、ポリシーテストでは `.PolicyId == policy.ID.String()` で比較した。

5. **medium 環境ホスト数**: `medium.yaml` の計算値は tokyo サイト (5×10×5=250) + osaka サイト (3×10×5=150) = 400 ホスト。テストの閾値は余裕を持たせ 100 以上とした。
