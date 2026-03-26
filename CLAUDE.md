## サーバー起動

- `make serve` でコントローラーを起動する（バックグラウンド実行される）
- `make serve-worker` でスタブワーカーを起動する
- 再度 `make serve` を実行すると、古いプロセスを自動で停止してから起動する
- ログは `make logs` で確認できる
- ポート番号を直接指定してはいけない
- サーバー起動スクリプトを作成・変更する場合は、portman ガイドを参照すること:
  https://raw.githubusercontent.com/tjst-t/port-manager/main/docs/CLAUDE_INTEGRATION.md
- .env ファイルを git commit してはいけない（.gitignore に追加すること）

## デザイン

- UIを実装する際は design-system リポジトリの DESIGN_SYSTEM.md に従うこと:
  https://raw.githubusercontent.com/tjst-t/design-system/main/DESIGN_SYSTEM.md
- 独自の色・フォント・スペーシングを使わない
- Tailwind使用時は design-system の tailwind.config.js のトークンを参照
