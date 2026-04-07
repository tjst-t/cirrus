import { test, expect } from '@playwright/test'

// このテストは BASE_URL 環境変数が設定されている場合のみ実行
// make serve で起動したサーバーに対して実行する統合テスト
test.skip(!process.env.BASE_URL, 'BASE_URL not set — skipping serve integration check')

test('WebUI is accessible via make serve', async ({ page }) => {
  await page.goto('/')
  // ログインページまたはダッシュボードが表示されていること
  await expect(page).toHaveURL(/\/(login)?$/)
})
