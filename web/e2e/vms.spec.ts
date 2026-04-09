import { test, expect } from '@playwright/test'

test.describe('VM 管理フロー', () => {
  test.beforeEach(async ({ page }) => {
    // addInitScript でリダイレクト前に localStorage をセット
    await page.addInitScript(() => {
      localStorage.setItem('cirrus_token', 'test-token')
      localStorage.setItem('cirrus_tenant_id', 'test-tenant-id')
    })

    // API をモック
    await page.route('/api/v1/**', async (route) => {
      const url = route.request().url()
      if (url.includes('/vms') && route.request().method() === 'GET') {
        await route.fulfill({ status: 200, json: { items: [], next_cursor: '' } })
      } else if (url.includes('/flavors')) {
        await route.fulfill({
          status: 200,
          json: { items: [
            { id: 'flavor-1', name: 'm1.small', vcpus: 1, ram_mb: 1024, disk_gb: 10 },
          ], next_cursor: '' },
        })
      } else if (url.includes('/networks')) {
        await route.fulfill({
          status: 200,
          json: { items: [
            { id: 'net-1', name: 'default', cidr: '10.0.0.0/24', status: 'active', created_at: '2024-01-01T00:00:00Z' },
          ], next_cursor: '' },
        })
      } else if (url.includes('/volume-types')) {
        await route.fulfill({ status: 200, json: [] })
      } else {
        await route.fulfill({ status: 200, json: { items: [], next_cursor: '' } })
      }
    })
  })

  test('VM 一覧ページが表示される', async ({ page }) => {
    await page.goto('/vms')
    // ページタイトル「VM 管理」が表示されること
    await expect(page.getByText('VM 管理')).toBeVisible()
  })

  test('VM 作成ボタンが存在する', async ({ page }) => {
    await page.goto('/vms')
    // 「+ VM を作成」ボタンが存在すること
    const createBtn = page.locator('button', { hasText: 'VM を作成' })
    await expect(createBtn).toBeVisible()
  })

  test('VM 一覧が空の場合は「VM がありません」と表示される', async ({ page }) => {
    await page.goto('/vms')
    await expect(page.getByText('VM がありません')).toBeVisible()
  })

  test('VM 作成ダイアログが開く', async ({ page }) => {
    await page.goto('/vms')

    const createBtn = page.locator('button', { hasText: 'VM を作成' })
    await createBtn.click()

    // ダイアログの見出し（h3）が表示されること
    await expect(page.locator('h3', { hasText: 'VM を作成' })).toBeVisible()
    // 名前入力フィールドが存在すること
    await expect(page.locator('input[placeholder="my-vm"]')).toBeVisible()
  })

  test('VM 作成ダイアログでキャンセルできる', async ({ page }) => {
    await page.goto('/vms')

    const createBtn = page.locator('button', { hasText: 'VM を作成' })
    await createBtn.click()

    // ダイアログの見出し（h3）が表示されること
    await expect(page.locator('h3', { hasText: 'VM を作成' })).toBeVisible()

    // キャンセルボタンでダイアログを閉じる
    await page.locator('button', { hasText: 'キャンセル' }).click()

    // ダイアログが閉じること
    await expect(page.locator('input[placeholder="my-vm"]')).not.toBeVisible()
  })
})
