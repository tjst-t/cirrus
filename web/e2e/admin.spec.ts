import { test, expect } from '@playwright/test'

test.describe('管理者 UI フロー', () => {
  test.beforeEach(async ({ page }) => {
    // addInitScript でリダイレクト前に localStorage をセット
    await page.addInitScript(() => {
      localStorage.setItem('cirrus_token', 'test-token')
      localStorage.setItem('cirrus_tenant_id', 'test-tenant-id')
    })

    // API をモック
    await page.route('/api/v1/**', async (route) => {
      const url = route.request().url()
      if (url.includes('/organizations')) {
        await route.fulfill({ status: 200, json: [] })
      } else if (url.includes('/hosts')) {
        await route.fulfill({ status: 200, json: [] })
      } else if (url.includes('/storage-domains')) {
        await route.fulfill({ status: 200, json: [] })
      } else if (url.includes('/storage-backends')) {
        await route.fulfill({ status: 200, json: [] })
      } else if (url.includes('/volume-types')) {
        await route.fulfill({ status: 200, json: [] })
      } else {
        await route.fulfill({ status: 200, json: [] })
      }
    })
  })

  test('組織一覧ページが表示される', async ({ page }) => {
    await page.goto('/admin/organizations')
    // ページタイトル「組織・テナント管理」が表示されること
    await expect(page.getByText('組織・テナント管理')).toBeVisible()
  })

  test('組織一覧が空の場合は「組織がありません」と表示される', async ({ page }) => {
    await page.goto('/admin/organizations')
    await expect(page.getByText('組織がありません')).toBeVisible()
  })

  test('組織作成ボタンが表示される', async ({ page }) => {
    await page.goto('/admin/organizations')
    const createBtn = page.locator('button', { hasText: '組織を作成' })
    await expect(createBtn).toBeVisible()
  })

  test('ホスト一覧ページが表示される', async ({ page }) => {
    await page.goto('/admin/hosts')
    // ページにホスト管理の見出し（h1）が表示されること
    await expect(page.locator('h1', { hasText: 'ホスト管理' })).toBeVisible()
  })

  test('ホスト一覧が空の場合は「ホストがありません」と表示される', async ({ page }) => {
    await page.goto('/admin/hosts')
    await expect(page.getByText('ホストがありません')).toBeVisible()
  })

  test('/admin にアクセスすると /admin/organizations にリダイレクトされる', async ({ page }) => {
    await page.goto('/admin')
    await expect(page).toHaveURL('/admin/organizations')
  })
})
