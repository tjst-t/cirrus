# S048-1 テスト結果

## 実行日時
2026-04-11

## テストファイル
`web/e2e/s048-network.spec.ts`

## 結果サマリー
**7 passed / 7 total** (6.3s)

## テスト詳細

| # | テスト名 | 結果 | 時間 |
|---|---------|------|------|
| 1 | ネットワーク一覧: ネットワークが存在しない場合に空状態を表示する | ✓ PASS | 533ms |
| 2 | ネットワーク作成: 名前とCIDRを入力して作成できる | ✓ PASS | 840ms |
| 3 | ネットワーク作成: CIDRを省略すると自動割り当てで作成できる | ✓ PASS | 618ms |
| 4 | ネットワーク削除: 確認後に削除される | ✓ PASS | 571ms |
| 5 | グループ管理: ネットワーク行を展開してグループを作成・削除できる | ✓ PASS | 830ms |
| 6 | ポリシー管理: src/dstグループを選択してポリシーを作成・削除できる | ✓ PASS | 898ms |
| 7 | ネットワーク一覧: API失敗時にエラーメッセージを表示する | ✓ PASS | 382ms |

## 実装変更点

### `web/src/api/networks.ts`
- `NetworkPolicy` 型をバックエンド実装に合わせて修正
  - 削除: `name`, `direction`, `port_range_min`, `port_range_max`, `remote_cidr`
  - 追加: `src_group_id`, `dst_group_id`, `dst_port`, `priority`
- `CreateNetworkPolicyRequest` 型を修正（PolicySpec に対応）
- `CreateNetworkRequest.cidr` を optional に変更
- `Network` 型に `tenant_id`, `vni`, `updated_at` を追加
- `NetworkGroup` から不要な `description` フィールドを削除

### `web/src/pages/tenant/NetworksPage.tsx`
- `data-testid` 属性をすべての要素に付与
- ポリシーパネルをグループ一覧と連携（src/dst グループ選択に既存グループを使用）
- グループ名解決: ポリシー行で UUID を名前に変換して表示
- グループ 0 件時はポリシー追加ボタンを `disabled`
- ネットワーク作成後の楽観的更新（optimistic update）実装
  - フォームの名前を即座にリストに反映してから非同期リロード
- CIDR フィールドを任意（required 削除）、placeholder を「省略可（自動割り当て）」に変更
