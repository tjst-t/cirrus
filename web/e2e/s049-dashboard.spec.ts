import { test, expect } from '@playwright/test'

// ── 定数 ────────────────────────────────────────────────────────────────────
const TENANT_ID = 'test-tenant-id'

const TENANT_1 = {
  id: TENANT_ID,
  name: 'Test Tenant',
  organization_id: 'org-1',
  created_at: '2024-01-01T00:00:00Z',
}

const QUOTA_FULL = {
  limits: {
    vcpus: 16,
    memory_mb: 32768,
    vm_count: 10,
    volume_gb: 1000,
    volumes: 20,
    snapshots: 40,
    networks: 5,
    egresses: 3,
    ingresses: 10,
  },
  usage: {
    tenant_id: TENANT_ID,
    vcpus_used: 4,
    memory_mb_used: 8192,
    vm_count_used: 2,
    volume_gb_used: 100,
    volumes_used: 3,
    snapshots_used: 1,
    networks_used: 1,
    egresses_used: 0,
    ingresses_used: 0,
  },
}

const QUOTA_AT_LIMIT = {
  limits: {
    vcpus: 4,
    memory_mb: 8192,
    vm_count: 2,
    volume_gb: 100,
    volumes: 3,
    snapshots: 1,
    networks: 1,
    egresses: 0,
    ingresses: 0,
  },
  usage: {
    tenant_id: TENANT_ID,
    vcpus_used: 4,
    memory_mb_used: 8192,
    vm_count_used: 2,
    volume_gb_used: 100,
    volumes_used: 3,
    snapshots_used: 1,
    networks_used: 1,
    egresses_used: 0,
    ingresses_used: 0,
  },
}

// ── 共通セットアップ ──────────────────────────────────────────────────────────
function setupAuth(page: Parameters<Parameters<typeof test>[1]>[0]['page']) {
  return page.addInitScript(() => {
    localStorage.setItem('cirrus_token', 'test-token')
    localStorage.setItem('cirrus_tenant_id', 'test-tenant-id')
  })
}

// ────────────────────────────────────────────────────────────────────────────
// S049-2: ダッシュボード
// ────────────────────────────────────────────────────────────────────────────
test.describe('S049-2: ダッシュボード', () => {
  test.beforeEach(async ({ page }) => {
    await setupAuth(page)
    await page.route('**/api/v1/me/tenants', (route) =>
      route.fulfill({ status: 200, json: { items: [TENANT_1], next_cursor: '' } })
    )
    await page.route('**/api/v1/me', (route) =>
      route.fulfill({ status: 200, json: { id: 'user-1', email: 'test@example.com' } })
    )
  })

  test('ダッシュボード: サマリカードと Quota バーを表示する', async ({ page }) => {
    await page.route(
      `**/api/v1/tenants/${TENANT_ID}/quota`,
      (route) => route.fulfill({ status: 200, json: QUOTA_FULL })
    )

    await page.goto('/')

    // サマリカード
    await expect(page.getByTestId('dashboard-vm-count')).toHaveText('2')
    await expect(page.getByTestId('dashboard-network-count')).toHaveText('1')
    await expect(page.getByTestId('dashboard-volume-gb')).toHaveText('100')
    await expect(page.getByTestId('dashboard-vcpu-usage')).toContainText('4')
    await expect(page.getByTestId('dashboard-vcpu-usage')).toContainText('16')
    await expect(page.getByTestId('dashboard-memory-usage')).toContainText('8')   // 8192 MB = 8 GB
    await expect(page.getByTestId('dashboard-memory-usage')).toContainText('32')  // 32768 MB = 32 GB

    // Quota バー（各リソース）
    await expect(page.getByTestId('quota-bar-vcpus')).toBeVisible()
    await expect(page.getByTestId('quota-bar-memory')).toBeVisible()
    await expect(page.getByTestId('quota-bar-vm-count')).toBeVisible()
    await expect(page.getByTestId('quota-bar-volume-gb')).toBeVisible()
    await expect(page.getByTestId('quota-bar-networks')).toBeVisible()
    await expect(page.getByTestId('quota-bar-volumes')).toBeVisible()
  })

  test('ダッシュボード: Quota 上限到達時にバーが100%表示になる', async ({ page }) => {
    await page.route(
      `**/api/v1/tenants/${TENANT_ID}/quota`,
      (route) => route.fulfill({ status: 200, json: QUOTA_AT_LIMIT })
    )

    await page.goto('/')
    await expect(page.getByTestId('quota-bar-vcpus')).toBeVisible()
    // 上限到達時はバーが警告色（aria-valuenow=100 または data-full="true"）
    await expect(page.getByTestId('quota-bar-vcpus')).toHaveAttribute('data-full', 'true')
  })

  test('ダッシュボード: quota API エラー時にエラーメッセージを表示する', async ({ page }) => {
    await page.route(
      `**/api/v1/tenants/${TENANT_ID}/quota`,
      (route) => route.fulfill({ status: 500, json: { error: 'internal server error' } })
    )

    await page.goto('/')
    await expect(page.getByTestId('dashboard-error-message')).toBeVisible()
    // サマリカードは表示しない
    await expect(page.getByTestId('dashboard-vm-count')).not.toBeVisible()
  })

  test('ダッシュボード: テナント未選択時に案内メッセージを表示する', async ({ page }) => {
    // tenantId を localStorage にセットしない
    await page.addInitScript(() => {
      localStorage.setItem('cirrus_token', 'test-token')
      // cirrus_tenant_id は設定しない
    })

    await page.goto('/')
    await expect(page.getByTestId('dashboard-no-tenant-message')).toBeVisible()
    await expect(page.getByTestId('dashboard-vm-count')).not.toBeVisible()
  })

  test('ダッシュボード: ローディング中はスケルトンまたはスピナーを表示する', async ({ page }) => {
    let resolve: () => void
    const delayed = new Promise<void>((r) => { resolve = r })

    await page.route(
      `**/api/v1/tenants/${TENANT_ID}/quota`,
      async (route) => {
        await delayed
        return route.fulfill({ status: 200, json: QUOTA_FULL })
      }
    )

    await page.goto('/')
    await expect(page.getByTestId('dashboard-loading')).toBeVisible()

    resolve!()
    await expect(page.getByTestId('dashboard-vm-count')).toBeVisible()
  })
})
