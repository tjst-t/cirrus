import { test, expect } from '@playwright/test'

// Fixtures
const ORG_1 = {
  id: 'org-aaaa-0000-0000-0000-000000000001',
  name: 'test-org',
  created_at: '2024-01-01T00:00:00Z',
}
const TENANT_1 = {
  id: 'ten-bbbb-0000-0000-0000-000000000001',
  name: 'test-tenant',
  organization_id: ORG_1.id,
  created_at: '2024-01-01T00:00:00Z',
}
const QUOTA_1 = {
  limits: {
    vcpus: 10,
    memory_mb: 20480,
    vm_count: 5,
    volume_gb: 500,
    networks: 3,
    egresses: 2,
    ingresses: 2,
  },
  usage: {
    vcpus_used: 4,
    memory_mb_used: 8192,
    vm_count_used: 2,
    volume_gb_used: 100,
    networks_used: 1,
    egresses_used: 0,
    ingresses_used: 0,
  },
}
const DRIFT_OPEN = {
  id: 'drift-cccc-0000-0000-0000-000000000001',
  resource_type: 'vm' as const,
  resource_id: 'vm-dddd-0000-0000-0000-000000000001',
  description: 'VM state mismatch: expected running, got stopped',
  status: 'open' as const,
  detected_at: '2024-06-01T10:00:00Z',
  resolved_at: null,
}
const DRIFT_RESOLVED = {
  ...DRIFT_OPEN,
  id: 'drift-cccc-0000-0000-0000-000000000002',
  status: 'resolved' as const,
  resolved_at: '2024-06-01T12:00:00Z',
}

function authInit(page: import('@playwright/test').Page) {
  return page.addInitScript(() => {
    localStorage.setItem('cirrus_token', 'test-token')
    localStorage.setItem('cirrus_tenant_id', 'test-tenant-id')
  })
}

function mockOrgTenant(page: import('@playwright/test').Page) {
  page.route('**/api/v1/organizations', (route) => {
    if (route.request().method() === 'GET') {
      route.fulfill({ status: 200, json: { items: [ORG_1], next_cursor: '' } })
    } else {
      route.continue()
    }
  })
  page.route(`**/api/v1/organizations/${ORG_1.id}/tenants`, (route) => {
    if (route.request().method() === 'GET') {
      route.fulfill({ status: 200, json: { items: [TENANT_1], next_cursor: '' } })
    } else {
      route.continue()
    }
  })
}

// ---------------------------------------------------------------------------
// S046-3: Quota 設定
// ---------------------------------------------------------------------------

test.describe('S046-3: Quota 設定', () => {
  test('テナントの Quota を表示できる', async ({ page }) => {
    await authInit(page)
    await mockOrgTenant(page)
    await page.route(`**/api/v1/tenants/${TENANT_1.id}/quota`, (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: QUOTA_1 })
      } else {
        route.continue()
      }
    })

    await page.goto('/admin/quotas')
    await page.getByTestId(`quota-edit-button-${TENANT_1.id}`).click()

    await expect(page.getByTestId(`quota-vcpus-${TENANT_1.id}`)).toHaveValue('10')
    await expect(page.getByTestId(`quota-memory-${TENANT_1.id}`)).toHaveValue('20480')
    await expect(page.getByTestId(`quota-vm-count-${TENANT_1.id}`)).toHaveValue('5')
    await expect(page.getByTestId(`quota-volume-gb-${TENANT_1.id}`)).toHaveValue('500')
    await expect(page.getByTestId(`quota-networks-${TENANT_1.id}`)).toHaveValue('3')
    await expect(page.getByTestId(`quota-egresses-${TENANT_1.id}`)).toHaveValue('2')
    await expect(page.getByTestId(`quota-ingresses-${TENANT_1.id}`)).toHaveValue('2')
  })

  test('Quota を変更して保存できる', async ({ page }) => {
    await authInit(page)
    await mockOrgTenant(page)

    let saved = false
    await page.route(`**/api/v1/tenants/${TENANT_1.id}/quota`, (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: QUOTA_1 })
      } else if (route.request().method() === 'PUT') {
        const body = route.request().postDataJSON()
        // The API sends flat QuotaLimits fields (no `limits` wrapper)
        expect(body.vcpus).toBeGreaterThanOrEqual(0)
        saved = true
        route.fulfill({ status: 200, json: { ...QUOTA_1, limits: { ...QUOTA_1.limits, vcpus: 20 } } })
      } else {
        route.continue()
      }
    })

    await page.goto('/admin/quotas')
    await page.getByTestId(`quota-edit-button-${TENANT_1.id}`).click()

    await page.getByTestId(`quota-vcpus-${TENANT_1.id}`).fill('20')
    await page.getByTestId(`quota-save-button-${TENANT_1.id}`).click()

    await expect(page.getByTestId(`quota-save-success-${TENANT_1.id}`)).toBeVisible()
    expect(saved).toBe(true)
  })

  test('Quota 保存失敗時にエラーが表示される', async ({ page }) => {
    await authInit(page)
    await mockOrgTenant(page)
    await page.route(`**/api/v1/tenants/${TENANT_1.id}/quota`, (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: QUOTA_1 })
      } else if (route.request().method() === 'PUT') {
        route.fulfill({ status: 500, json: { error: 'internal error' } })
      } else {
        route.continue()
      }
    })

    await page.goto('/admin/quotas')
    await page.getByTestId(`quota-edit-button-${TENANT_1.id}`).click()
    await page.getByTestId(`quota-save-button-${TENANT_1.id}`).click()

    await expect(page.getByTestId(`quota-save-error-${TENANT_1.id}`)).toBeVisible()
  })

  test('組織がない場合は空状態が表示される', async ({ page }) => {
    await authInit(page)
    await page.route('**/api/v1/organizations', (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: { items: [], next_cursor: '' } })
      } else {
        route.continue()
      }
    })

    await page.goto('/admin/quotas')
    await expect(page.getByTestId('empty-orgs-quota')).toBeVisible()
  })
})

// ---------------------------------------------------------------------------
// S046-3: Drift Event ビューア
// ---------------------------------------------------------------------------

test.describe('S046-3: Drift Event ビューア', () => {
  test('Drift Event 一覧が表示される', async ({ page }) => {
    await authInit(page)
    await page.route('**/api/v1/admin/drift-events*', (route) => {
      // *  == wildcard for query params (?status=... etc)
      route.fulfill({ status: 200, json: { items: [DRIFT_OPEN, DRIFT_RESOLVED], next_cursor: '' } })
    })

    await page.goto('/admin/drift-events')
    await expect(page.getByTestId(`drift-row-${DRIFT_OPEN.id}`)).toBeVisible()
    await expect(page.getByTestId(`drift-row-${DRIFT_RESOLVED.id}`)).toBeVisible()
  })

  test('イベントがない場合は空状態が表示される', async ({ page }) => {
    await authInit(page)
    await page.route('**/api/v1/admin/drift-events*', (route) => {
      route.fulfill({ status: 200, json: { items: [], next_cursor: '' } })
    })

    await page.goto('/admin/drift-events')
    await expect(page.getByTestId('empty-drift-events')).toBeVisible()
  })

  test('resource_type フィルタで絞り込める', async ({ page }) => {
    await authInit(page)
    let filteredType: string | null = null

    await page.route('**/api/v1/admin/drift-events*', (route) => {
      const url = new URL(route.request().url())
      filteredType = url.searchParams.get('resource_type')
      const items = filteredType === 'vm' ? [DRIFT_OPEN] : [DRIFT_OPEN, DRIFT_RESOLVED]
      route.fulfill({ status: 200, json: { items, next_cursor: '' } })
    })

    await page.goto('/admin/drift-events')
    await page.getByTestId('drift-filter-resource-type').selectOption('vm')

    await expect(page.getByTestId(`drift-row-${DRIFT_OPEN.id}`)).toBeVisible()
    expect(filteredType).toBe('vm')
  })

  test('status フィルタで open イベントのみ表示できる', async ({ page }) => {
    await authInit(page)
    let filteredStatus: string | null = null

    await page.route('**/api/v1/admin/drift-events*', (route) => {
      const url = new URL(route.request().url())
      filteredStatus = url.searchParams.get('status')
      const items = filteredStatus === 'open' ? [DRIFT_OPEN] : [DRIFT_OPEN, DRIFT_RESOLVED]
      route.fulfill({ status: 200, json: { items, next_cursor: '' } })
    })

    await page.goto('/admin/drift-events')
    await page.getByTestId('drift-filter-status').selectOption('open')

    await expect(page.getByTestId(`drift-row-${DRIFT_OPEN.id}`)).toBeVisible()
    await expect(page.getByTestId(`drift-row-${DRIFT_RESOLVED.id}`)).not.toBeVisible()
    expect(filteredStatus).toBe('open')
  })

  test('open イベントを解決済みにできる', async ({ page }) => {
    await authInit(page)
    let resolved = false

    await page.route('**/api/v1/admin/drift-events*', (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({
          status: 200,
          json: {
            items: [{ ...DRIFT_OPEN, status: resolved ? 'resolved' : 'open' }],
            next_cursor: '',
          },
        })
      } else {
        route.continue()
      }
    })
    await page.route(`**/api/v1/admin/drift-events/${DRIFT_OPEN.id}`, (route) => {
      if (route.request().method() === 'PATCH') {
        resolved = true
        route.fulfill({ status: 200, json: { ...DRIFT_OPEN, status: 'resolved', resolved_at: '2024-06-01T12:00:00Z' } })
      } else {
        route.continue()
      }
    })

    await page.goto('/admin/drift-events')
    await page.getByTestId(`resolve-drift-button-${DRIFT_OPEN.id}`).click()

    await expect(page.getByTestId(`drift-status-${DRIFT_OPEN.id}`)).toContainText('resolved')
    expect(resolved).toBe(true)
  })

  test('resolved イベントには解決ボタンが表示されない', async ({ page }) => {
    await authInit(page)
    await page.route('**/api/v1/admin/drift-events*', (route) => {
      route.fulfill({ status: 200, json: { items: [DRIFT_RESOLVED], next_cursor: '' } })
    })

    await page.goto('/admin/drift-events')
    await expect(page.getByTestId(`resolve-drift-button-${DRIFT_RESOLVED.id}`)).not.toBeVisible()
  })
})
