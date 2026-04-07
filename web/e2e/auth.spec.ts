import { test, expect } from '@playwright/test'

test.describe('認証フロー', () => {
  test.beforeEach(async ({ page }) => {
    // API をモックして実際のバックエンドなしでテスト可能にする
    await page.route('/api/v1/**', async (route) => {
      const url = route.request().url()
      if (url.includes('/organizations')) {
        await route.fulfill({ status: 200, json: [] })
      } else {
        await route.fulfill({ status: 200, json: [] })
      }
    })
  })

  test('ログインページが表示される', async ({ page }) => {
    await page.goto('/login')
    // トークン入力フォームが表示されること
    await expect(page.locator('input#token')).toBeVisible()
    await expect(page.locator('button[type="submit"]')).toBeVisible()
    await expect(page.locator('label[for="token"]')).toBeVisible()
  })

  test('有効なトークンでログインするとダッシュボードにリダイレクトされる', async ({ page }) => {
    await page.goto('/login')

    // トークンを入力してログイン
    await page.fill('input#token', 'test-token')
    await page.click('button[type="submit"]')

    // ダッシュボード（/）にリダイレクトされること
    await expect(page).toHaveURL('/')
  })

  test('無効なトークンでエラーが表示される', async ({ page }) => {
    // 401 テスト用: unrouteAll() を使わず、より具体的なルートを後から登録（Playwright は後から登録が優先）
    await page.route('/api/v1/organizations', async (route) => {
      await route.fulfill({ status: 401, json: { error: 'unauthorized' } })
    })

    await page.goto('/login')
    await page.fill('input#token', 'invalid-token')
    await page.click('button[type="submit"]')

    // エラーメッセージが表示されること
    await expect(page.getByText('トークンが無効です')).toBeVisible()
  })

  test('未認証状態で保護ルートにアクセスすると /login にリダイレクトされる', async ({ page }) => {
    // addInitScript で localStorage を空にした状態でページ遷移
    await page.addInitScript(() => {
      localStorage.removeItem('cirrus_token')
      localStorage.removeItem('cirrus_tenant_id')
    })

    await page.goto('/')
    await expect(page).toHaveURL('/login')
  })
})
