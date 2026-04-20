import { test, expect } from '@playwright/test'

// --- Fixtures ---
const TENANT_ID = 'aaaaaaaa-0000-0000-0000-000000000001'

const FLAVOR_1 = {
  id: 'fl-1111-0000-0000-0000-000000000001',
  name: 'm1.small',
  vcpus: 1,
  ram_mb: 1024,
  disk_gb: 20,
  is_public: true,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
}

const NETWORK_1 = {
  id: 'net-1111-0000-0000-0000-000000000001',
  tenant_id: TENANT_ID,
  name: 'default-net',
  cidr: '10.0.0.0/24',
  vni: 100,
  status: 'active',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
}

const AZ_1 = {
  id: 'az-1111-0000-0000-0000-000000000001',
  name: 'az1',
  description: '',
  location_id: 'loc-0000-0000-0000-000000000001',
  enabled: true,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
}

const VM_ERROR_STATUS = {
  id: 'vm-eeee-0000-0000-0000-000000000001',
  tenant_id: TENANT_ID,
  name: 'broken-vm',
  flavor_id: FLAVOR_1.id,
  az_id: AZ_1.id,
  status: 'error',
  error_message: 'ホストへの接続が切断されました',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
}

// --- Helpers ---
function authInit(page: import('@playwright/test').Page) {
  return page.addInitScript(() => {
    localStorage.setItem('cirrus_token', 'test-token')
    localStorage.setItem('cirrus_tenant_id', 'aaaaaaaa-0000-0000-0000-000000000001')
  })
}

/** Mock すべての補助エンドポイント（vms 以外）を設定する */
function mockSupportingEndpoints(page: import('@playwright/test').Page) {
  page.route('**/api/v1/flavors', (route) =>
    route.fulfill({ status: 200, json: { items: [FLAVOR_1], next_cursor: '' } })
  )
  page.route('**/api/v1/networks', (route) =>
    route.fulfill({ status: 200, json: { items: [NETWORK_1], next_cursor: '' } })
  )
  // availability-zones と volume-types はバックエンドがプレーン配列を返す
  page.route('**/api/v1/availability-zones', (route) =>
    route.fulfill({ status: 200, json: [AZ_1] })
  )
  page.route('**/api/v1/volume-types', (route) =>
    route.fulfill({ status: 200, json: [] })
  )
}

/** VM作成ダイアログを開いて必須フィールドを入力する（flavor・network はデフォルト選択） */
async function openAndFillCreateDialog(page: import('@playwright/test').Page) {
  await page.getByTestId('vm-create-button').click()
  await page.getByTestId('vm-create-name').fill('test-vm')
}

// --- Tests ---
test.describe('S051-2: GUI エラーメッセージ表示', () => {
  test('VM作成: ERR_NO_HOST でホスト不足の日本語メッセージが表示される', async ({ page }) => {
    await authInit(page)
    mockSupportingEndpoints(page)

    let createCalled = false
    page.route('**/api/v1/vms', (route) => {
      if (route.request().method() === 'POST') {
        const body = route.request().postDataJSON()
        // S051-1 が実装する構造化エラーレスポンスを返す
        expect(body.name).toBeTruthy()
        expect(body.flavor_id).toBeTruthy()
        expect(body.network_id).toBeTruthy()
        createCalled = true
        route.fulfill({
          status: 422,
          json: { code: 'ERR_NO_HOST', message: 'No available host', detail: {} },
        })
      } else {
        route.fulfill({ status: 200, json: { items: [], next_cursor: '' } })
      }
    })

    await page.goto('/vms')
    await openAndFillCreateDialog(page)
    await page.getByTestId('vm-create-submit').click()

    await expect(page.getByTestId('form-error')).toBeVisible()
    await expect(page.getByTestId('form-error')).toContainText('利用可能なホストがありません')
    expect(createCalled).toBe(true)
  })

  test('VM作成: ERR_QUOTA_VCPU で detail のクォータ数値が動的補完される', async ({ page }) => {
    await authInit(page)
    mockSupportingEndpoints(page)

    let createCalled = false
    page.route('**/api/v1/vms', (route) => {
      if (route.request().method() === 'POST') {
        createCalled = true
        route.fulfill({
          status: 422,
          json: {
            code: 'ERR_QUOTA_VCPU',
            message: 'Quota exceeded',
            detail: { resource: 'vcpu', limit: 8, requested: 2, current: 7 },
          },
        })
      } else {
        route.fulfill({ status: 200, json: { items: [], next_cursor: '' } })
      }
    })

    await page.goto('/vms')
    await openAndFillCreateDialog(page)
    await page.getByTestId('vm-create-submit').click()

    await expect(page.getByTestId('form-error')).toBeVisible()
    await expect(page.getByTestId('form-error')).toContainText('クォータ上限に達しています')
    await expect(page.getByTestId('form-error')).toContainText('vcpu: 7/8')
    expect(createCalled).toBe(true)
  })

  test('VM作成: ERR_CONFLICT で名前重複の日本語メッセージが表示される', async ({ page }) => {
    await authInit(page)
    mockSupportingEndpoints(page)

    let createCalled = false
    page.route('**/api/v1/vms', (route) => {
      if (route.request().method() === 'POST') {
        createCalled = true
        route.fulfill({
          status: 409,
          json: { code: 'ERR_CONFLICT', message: 'Resource already exists', detail: {} },
        })
      } else {
        route.fulfill({ status: 200, json: { items: [], next_cursor: '' } })
      }
    })

    await page.goto('/vms')
    await openAndFillCreateDialog(page)
    await page.getByTestId('vm-create-submit').click()

    await expect(page.getByTestId('form-error')).toBeVisible()
    await expect(page.getByTestId('form-error')).toContainText('同じ名前のリソースが既に存在します')
    expect(createCalled).toBe(true)
  })

  test('VM作成: 未知エラーコードは message フィールドをそのままフォールバック表示する', async ({ page }) => {
    await authInit(page)
    mockSupportingEndpoints(page)

    page.route('**/api/v1/vms', (route) => {
      if (route.request().method() === 'POST') {
        route.fulfill({
          status: 500,
          json: { code: 'ERR_INTERNAL', message: '予期しないエラーが発生しました', detail: {} },
        })
      } else {
        route.fulfill({ status: 200, json: { items: [], next_cursor: '' } })
      }
    })

    await page.goto('/vms')
    await openAndFillCreateDialog(page)
    await page.getByTestId('vm-create-submit').click()

    await expect(page.getByTestId('form-error')).toBeVisible()
    await expect(page.getByTestId('form-error')).toContainText('予期しないエラーが発生しました')
  })

  test('VM一覧: error ステータスの VM にホバーするとエラーメッセージがツールチップで表示される', async ({
    page,
  }) => {
    await authInit(page)
    mockSupportingEndpoints(page)

    page.route('**/api/v1/vms', (route) =>
      route.fulfill({ status: 200, json: { items: [VM_ERROR_STATUS], next_cursor: '' } })
    )

    await page.goto('/vms')
    await expect(page.getByTestId(`vm-row-${VM_ERROR_STATUS.id}`)).toBeVisible()
    await expect(page.getByTestId(`vm-status-badge-${VM_ERROR_STATUS.id}`)).toContainText('エラー')

    await page.getByTestId(`vm-error-tooltip-trigger-${VM_ERROR_STATUS.id}`).hover()
    await expect(page.getByTestId(`vm-error-tooltip-content-${VM_ERROR_STATUS.id}`)).toBeVisible()
    await expect(
      page.getByTestId(`vm-error-tooltip-content-${VM_ERROR_STATUS.id}`)
    ).toContainText('ホストへの接続が切断されました')
  })
})
