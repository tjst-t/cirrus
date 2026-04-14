import { test, expect } from '@playwright/test'

// ── 定数 ────────────────────────────────────────────────────────────────────
const TENANT_ID = 'test-tenant-id'
const NET_ID = 'net-0000-0000-0000-000000000001'
const EGRESS_ID = 'egr-0000-0000-0000-000000000001'
const INGRESS_ID = 'ing-0000-0000-0000-000000000001'
const POOL_ID = 'pool-0000-0000-0000-000000000001'
const VM_ID = 'vm-0000-0000-0000-000000000001'

const TENANT_1 = {
  id: TENANT_ID,
  name: 'Test Tenant',
  organization_id: 'org-1',
  created_at: '2024-01-01T00:00:00Z',
}

const NET_1 = {
  id: NET_ID,
  tenant_id: TENANT_ID,
  name: 'my-network',
  cidr: '10.0.0.0/24',
  vni: 1001,
  status: 'active',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
}

// バックエンド Egress 構造体に合わせた正しい型
const EGRESS_1 = {
  id: EGRESS_ID,
  network_id: NET_ID,
  type: 'nat_gateway',
  config: {
    public_ip: '203.0.113.1',
  },
}

// バックエンド Ingress 構造体に合わせた正しい型
const INGRESS_1 = {
  id: INGRESS_ID,
  network_id: NET_ID,
  type: 'direct_ip',
  public_ip: '203.0.113.2',
  ip_pool_id: POOL_ID,
  config: {
    target_vm_id: VM_ID,
    target_ip: '10.0.0.1',
  },
  created_at: '2024-01-01T00:00:00Z',
}

const POOL_1 = {
  id: POOL_ID,
  name: 'public-pool',
  cidr: '203.0.113.0/24',
  description: 'パブリック IP プール',
  created_at: '2024-01-01T00:00:00Z',
}

const VM_1 = {
  id: VM_ID,
  name: 'web-server',
  tenant_id: TENANT_ID,
  status: 'running',
  vcpus: 2,
  memory_mb: 2048,
  created_at: '2024-01-01T00:00:00Z',
}

// ── 共通セットアップ ──────────────────────────────────────────────────────────
function setupAuth(page: Parameters<Parameters<typeof test>[1]>[0]['page']) {
  return page.addInitScript(() => {
    localStorage.setItem('cirrus_token', 'test-token')
    localStorage.setItem('cirrus_tenant_id', 'test-tenant-id')
  })
}

// ────────────────────────────────────────────────────────────────────────────
// S049-1-A: Egress 管理
// ────────────────────────────────────────────────────────────────────────────
test.describe('S049-1-A: Egress 管理', () => {
  test.beforeEach(async ({ page }) => {
    await setupAuth(page)
    await page.route('**/api/v1/me/tenants', (route) =>
      route.fulfill({ status: 200, json: { items: [TENANT_1], next_cursor: '' } })
    )
    await page.route('**/api/v1/me', (route) =>
      route.fulfill({ status: 200, json: { id: 'user-1', email: 'test@example.com' } })
    )
    await page.route('**/api/v1/networks', (route) =>
      route.fulfill({ status: 200, json: { items: [NET_1], next_cursor: '' } })
    )
  })

  test('Egress 一覧: ネットワーク未選択時またはegress 0件で空状態を表示する', async ({ page }) => {
    await page.route(
      `**/api/v1/tenants/${TENANT_ID}/networks/${NET_ID}/egresses`,
      (route) => route.fulfill({ status: 200, json: [] })
    )
    await page.goto('/egress')
    await expect(page.getByTestId('egress-empty-state')).toBeVisible()
    await expect(page.getByTestId('egress-create-button')).toBeEnabled()
  })

  test('Egress 一覧: egress が存在する場合にテーブルで表示する', async ({ page }) => {
    await page.route(
      `**/api/v1/tenants/${TENANT_ID}/networks/${NET_ID}/egresses`,
      (route) => route.fulfill({ status: 200, json: [EGRESS_1] })
    )
    await page.goto('/egress')
    await expect(page.getByTestId(`egress-row-${EGRESS_ID}`)).toBeVisible()
    await expect(page.getByTestId(`egress-type-${EGRESS_ID}`)).toHaveText('nat_gateway')
    await expect(page.getByTestId(`egress-public-ip-${EGRESS_ID}`)).toHaveText('203.0.113.1')
  })

  test('Egress 作成: NAT ゲートウェイを作成して一覧に反映される', async ({ page }) => {
    let created = false
    await page.route(
      `**/api/v1/tenants/${TENANT_ID}/networks/${NET_ID}/egresses`,
      async (route) => {
        if (route.request().method() === 'GET') {
          return route.fulfill({ status: 200, json: created ? [EGRESS_1] : [] })
        }
        if (route.request().method() === 'POST') {
          const body = route.request().postDataJSON()
          // バックエンドが期待する形式: {type, config}
          expect(body.type).toBe('nat_gateway')
          created = true
          return route.fulfill({ status: 201, json: EGRESS_1 })
        }
      }
    )

    await page.goto('/egress')
    await expect(page.getByTestId('egress-empty-state')).toBeVisible()

    await page.getByTestId('egress-create-button').click()
    await expect(page.getByTestId('egress-create-dialog')).toBeVisible()
    await expect(page.getByTestId('egress-type-select')).toBeVisible()

    // type=nat_gateway はデフォルト選択済み
    await page.getByTestId('egress-create-submit').click()

    await expect(page.getByTestId(`egress-row-${EGRESS_ID}`)).toBeVisible()
  })

  test('Egress 作成失敗: エラーメッセージを表示する', async ({ page }) => {
    await page.route(
      `**/api/v1/tenants/${TENANT_ID}/networks/${NET_ID}/egresses`,
      async (route) => {
        if (route.request().method() === 'GET') {
          return route.fulfill({ status: 200, json: [] })
        }
        return route.fulfill({ status: 500, json: { error: 'internal server error' } })
      }
    )

    await page.goto('/egress')
    await page.getByTestId('egress-create-button').click()
    await page.getByTestId('egress-create-submit').click()
    await expect(page.getByTestId('egress-error-message')).toBeVisible()
  })

  test('Egress 削除: 確認後に削除され一覧から消える', async ({ page }) => {
    let deleted = false
    await page.route(
      `**/api/v1/tenants/${TENANT_ID}/networks/${NET_ID}/egresses`,
      (route) => route.fulfill({ status: 200, json: deleted ? [] : [EGRESS_1] })
    )
    await page.route(
      `**/api/v1/tenants/${TENANT_ID}/networks/${NET_ID}/egresses/${EGRESS_ID}`,
      (route) => {
        deleted = true
        return route.fulfill({ status: 204 })
      }
    )

    await page.goto('/egress')
    await expect(page.getByTestId(`egress-row-${EGRESS_ID}`)).toBeVisible()

    await page.getByTestId(`egress-delete-button-${EGRESS_ID}`).click()
    await expect(page.getByTestId(`egress-delete-confirm-${EGRESS_ID}`)).toBeVisible()
    await page.getByTestId(`egress-delete-confirm-${EGRESS_ID}`).click()

    await expect(page.getByTestId(`egress-row-${EGRESS_ID}`)).not.toBeVisible()
  })

  test('Egress 削除キャンセル: キャンセルしても一覧に残る', async ({ page }) => {
    await page.route(
      `**/api/v1/tenants/${TENANT_ID}/networks/${NET_ID}/egresses`,
      (route) => route.fulfill({ status: 200, json: [EGRESS_1] })
    )

    await page.goto('/egress')
    await page.getByTestId(`egress-delete-button-${EGRESS_ID}`).click()
    await expect(page.getByTestId(`egress-delete-confirm-${EGRESS_ID}`)).toBeVisible()
    await page.getByTestId(`egress-delete-cancel-${EGRESS_ID}`).click()
    await expect(page.getByTestId(`egress-row-${EGRESS_ID}`)).toBeVisible()
  })

  test('Egress 一覧: API エラー時にエラーメッセージを表示する', async ({ page }) => {
    await page.route(
      `**/api/v1/tenants/${TENANT_ID}/networks/${NET_ID}/egresses`,
      (route) => route.fulfill({ status: 500, json: { error: 'internal server error' } })
    )
    await page.goto('/egress')
    await expect(page.getByTestId('egress-error-message')).toBeVisible()
  })

  test('Egress 一覧: ネットワークが存在しない場合に案内メッセージを表示する', async ({ page }) => {
    // networks が空
    await page.route('**/api/v1/networks', (route) =>
      route.fulfill({ status: 200, json: { items: [], next_cursor: '' } })
    )
    await page.goto('/egress')
    await expect(page.getByTestId('egress-no-network-message')).toBeVisible()
  })
})

// ────────────────────────────────────────────────────────────────────────────
// S049-1-B: Ingress 管理
// ────────────────────────────────────────────────────────────────────────────
test.describe('S049-1-B: Ingress 管理', () => {
  test.beforeEach(async ({ page }) => {
    await setupAuth(page)
    await page.route('**/api/v1/me/tenants', (route) =>
      route.fulfill({ status: 200, json: { items: [TENANT_1], next_cursor: '' } })
    )
    await page.route('**/api/v1/me', (route) =>
      route.fulfill({ status: 200, json: { id: 'user-1', email: 'test@example.com' } })
    )
    await page.route('**/api/v1/networks', (route) =>
      route.fulfill({ status: 200, json: { items: [NET_1], next_cursor: '' } })
    )
    await page.route('**/api/v1/admin/ip-pools', (route) =>
      route.fulfill({ status: 200, json: [POOL_1] })
    )
    await page.route('**/api/v1/tenants/*/vms', (route) =>
      route.fulfill({ status: 200, json: { items: [VM_1], next_cursor: '' } })
    )
  })

  test('Ingress 一覧: ingress 0件で空状態を表示する', async ({ page }) => {
    await page.route(
      `**/api/v1/networks/${NET_ID}/ingresses`,
      (route) => route.fulfill({ status: 200, json: [] })
    )
    await page.goto('/ingress')
    await expect(page.getByTestId('ingress-empty-state')).toBeVisible()
    await expect(page.getByTestId('ingress-create-button')).toBeEnabled()
  })

  test('Ingress 一覧: ingress が存在する場合にテーブルで表示する', async ({ page }) => {
    await page.route(
      `**/api/v1/networks/${NET_ID}/ingresses`,
      (route) => route.fulfill({ status: 200, json: [INGRESS_1] })
    )
    await page.goto('/ingress')
    await expect(page.getByTestId(`ingress-row-${INGRESS_ID}`)).toBeVisible()
    await expect(page.getByTestId(`ingress-public-ip-${INGRESS_ID}`)).toHaveText('203.0.113.2')
    await expect(page.getByTestId(`ingress-type-${INGRESS_ID}`)).toHaveText('direct_ip')
  })

  test('Ingress 作成: フォームに入力して作成できる', async ({ page }) => {
    let created = false
    await page.route(
      `**/api/v1/networks/${NET_ID}/ingresses`,
      async (route) => {
        if (route.request().method() === 'GET') {
          return route.fulfill({ status: 200, json: created ? [INGRESS_1] : [] })
        }
        if (route.request().method() === 'POST') {
          const body = route.request().postDataJSON()
          // バックエンドが期待する形式: {type, public_ip, ip_pool_id, config}
          expect(body.type).toBe('direct_ip')
          expect(body.public_ip).toBeTruthy()
          expect(body.ip_pool_id).toBe(POOL_ID)
          created = true
          return route.fulfill({ status: 201, json: INGRESS_1 })
        }
      }
    )

    await page.goto('/ingress')
    await expect(page.getByTestId('ingress-empty-state')).toBeVisible()

    await page.getByTestId('ingress-create-button').click()
    await expect(page.getByTestId('ingress-create-dialog')).toBeVisible()

    // IP プール選択
    await page.getByTestId('ingress-ip-pool-select').selectOption(POOL_ID)
    // パブリック IP 入力
    await page.getByTestId('ingress-public-ip-input').fill('203.0.113.2')
    // ターゲット VM 選択（任意）
    await page.getByTestId('ingress-target-vm-select').selectOption(VM_ID)

    await page.getByTestId('ingress-create-submit').click()
    await expect(page.getByTestId(`ingress-row-${INGRESS_ID}`)).toBeVisible()
  })

  test('Ingress 作成: public_ip 未入力でバリデーションエラー', async ({ page }) => {
    await page.route(
      `**/api/v1/networks/${NET_ID}/ingresses`,
      (route) => route.fulfill({ status: 200, json: [] })
    )

    await page.goto('/ingress')
    await page.getByTestId('ingress-create-button').click()
    await page.getByTestId('ingress-ip-pool-select').selectOption(POOL_ID)
    // public_ip を入力しないで送信
    await page.getByTestId('ingress-create-submit').click()
    await expect(page.getByTestId('ingress-error-message')).toBeVisible()
  })

  test('Ingress 削除: 確認後に削除され一覧から消える', async ({ page }) => {
    let deleted = false
    await page.route(
      `**/api/v1/networks/${NET_ID}/ingresses`,
      (route) => route.fulfill({ status: 200, json: deleted ? [] : [INGRESS_1] })
    )
    await page.route(
      `**/api/v1/networks/${NET_ID}/ingresses/${INGRESS_ID}`,
      (route) => {
        deleted = true
        return route.fulfill({ status: 204 })
      }
    )

    await page.goto('/ingress')
    await expect(page.getByTestId(`ingress-row-${INGRESS_ID}`)).toBeVisible()

    await page.getByTestId(`ingress-delete-button-${INGRESS_ID}`).click()
    await expect(page.getByTestId(`ingress-delete-confirm-${INGRESS_ID}`)).toBeVisible()
    await page.getByTestId(`ingress-delete-confirm-${INGRESS_ID}`).click()

    await expect(page.getByTestId(`ingress-row-${INGRESS_ID}`)).not.toBeVisible()
  })

  test('Ingress 削除キャンセル: キャンセルしても一覧に残る', async ({ page }) => {
    await page.route(
      `**/api/v1/networks/${NET_ID}/ingresses`,
      (route) => route.fulfill({ status: 200, json: [INGRESS_1] })
    )

    await page.goto('/ingress')
    await page.getByTestId(`ingress-delete-button-${INGRESS_ID}`).click()
    await page.getByTestId(`ingress-delete-cancel-${INGRESS_ID}`).click()
    await expect(page.getByTestId(`ingress-row-${INGRESS_ID}`)).toBeVisible()
  })

  test('Ingress 一覧: API エラー時にエラーメッセージを表示する', async ({ page }) => {
    await page.route(
      `**/api/v1/networks/${NET_ID}/ingresses`,
      (route) => route.fulfill({ status: 500, json: { error: 'internal server error' } })
    )
    await page.goto('/ingress')
    await expect(page.getByTestId('ingress-error-message')).toBeVisible()
  })
})
