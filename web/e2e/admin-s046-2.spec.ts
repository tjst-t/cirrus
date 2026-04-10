import { test, expect } from '@playwright/test'

// Fixtures
const HOST_ACTIVE = {
  id: 'host-aaaa-0000-0000-0000-000000000001',
  name: 'host-01',
  address: '192.168.1.10',
  operational_state: 'active' as const,
  capability: {},
  resource_physical: { vcpus: 32, memory_mb: 65536 },
  overcommit_ratios: {},
  resource_used: { vcpus: 4, memory_mb: 8192 },
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
}
const HOST_MAINTENANCE = { ...HOST_ACTIVE, id: 'host-aaaa-0000-0000-0000-000000000002', name: 'host-02', operational_state: 'maintenance' as const }

const STORAGE_DOMAIN_1 = { id: 'sd-1111', name: 'default-sd', created_at: '2024-01-01T00:00:00Z' }
const BACKEND_1 = { id: 'bk-1111', storage_domain_id: STORAGE_DOMAIN_1.id, name: 'ceph-01', driver: 'ceph', endpoint: 'ceph://mon:6789/pool', total_capacity_gb: 0, total_iops: 0, capabilities: null, driver_config: {}, state: 'active', created_at: '2024-01-01T00:00:00Z', updated_at: '2024-01-01T00:00:00Z' }
const VOLTYPE_1 = { id: 'vt-1111', name: 'ssd', backend_id: BACKEND_1.id, created_at: '2024-01-01T00:00:00Z' }
const FLAVOR_1 = { id: 'fl-1111', name: 'm1.small', vcpus: 1, ram_mb: 1024, disk_gb: 20, created_at: '2024-01-01T00:00:00Z' }

const GW_NODE_1 = {
  id: 'gw-1111-0000-0000-0000-000000000001',
  host_id: HOST_ACTIVE.id,
  external_ip: '203.0.113.1',
  internal_ip: '10.0.0.1',
  status: 'active',
  created_at: '2024-01-01T00:00:00Z',
}
const IP_POOL_1 = {
  id: 'pool-1111-0000-0000-0000-000000000001',
  name: 'public-pool',
  cidr: '203.0.113.0/24',
  description: '',
  created_at: '2024-01-01T00:00:00Z',
}

function authInit(page: import('@playwright/test').Page) {
  return page.addInitScript(() => {
    localStorage.setItem('cirrus_token', 'test-token')
    localStorage.setItem('cirrus_tenant_id', 'test-tenant-id')
  })
}

// ---------------------------------------------------------------------------
// S046-2: ホスト一覧 / 状態遷移
// ---------------------------------------------------------------------------

test.describe('S046-2: ホスト管理', () => {
  test('ホスト一覧が表示される', async ({ page }) => {
    await authInit(page)
    await page.route('**/api/v1/hosts', (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: { items: [HOST_ACTIVE], next_cursor: '' } })
      } else {
        route.continue()
      }
    })

    await page.goto('/admin/hosts')
    await expect(page.getByTestId(`host-row-${HOST_ACTIVE.id}`)).toBeVisible()
    await expect(page.getByTestId(`host-status-${HOST_ACTIVE.id}`)).toContainText('アクティブ')
  })

  test('空のホスト一覧が表示される', async ({ page }) => {
    await authInit(page)
    await page.route('**/api/v1/hosts', (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: { items: [], next_cursor: '' } })
      } else {
        route.continue()
      }
    })

    await page.goto('/admin/hosts')
    await expect(page.getByTestId('empty-hosts')).toBeVisible()
  })

  test('active ホストに drain / maintenance ボタンが表示される', async ({ page }) => {
    await authInit(page)
    await page.route('**/api/v1/hosts', (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: { items: [HOST_ACTIVE], next_cursor: '' } })
      } else {
        route.continue()
      }
    })

    await page.goto('/admin/hosts')
    await expect(page.getByTestId(`host-action-drain-${HOST_ACTIVE.id}`)).toBeVisible()
    await expect(page.getByTestId(`host-action-maintenance-${HOST_ACTIVE.id}`)).toBeVisible()
    await expect(page.getByTestId(`host-action-activate-${HOST_ACTIVE.id}`)).not.toBeVisible()
    await expect(page.getByTestId(`host-action-retire-${HOST_ACTIVE.id}`)).not.toBeVisible()
  })

  test('maintenance ホストに activate / retire ボタンが表示される', async ({ page }) => {
    await authInit(page)
    await page.route('**/api/v1/hosts', (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: { items: [HOST_MAINTENANCE], next_cursor: '' } })
      } else {
        route.continue()
      }
    })

    await page.goto('/admin/hosts')
    await expect(page.getByTestId(`host-action-activate-${HOST_MAINTENANCE.id}`)).toBeVisible()
    await expect(page.getByTestId(`host-action-retire-${HOST_MAINTENANCE.id}`)).toBeVisible()
  })

  test('drain アクション後にステータスが更新される', async ({ page }) => {
    await authInit(page)

    let state: 'active' | 'draining' = 'active'
    await page.route('**/api/v1/hosts', (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({
          status: 200,
          json: { items: [{ ...HOST_ACTIVE, operational_state: state }], next_cursor: '' },
        })
      } else {
        route.continue()
      }
    })
    await page.route(`**/api/v1/hosts/${HOST_ACTIVE.id}/actions`, (route) => {
      state = 'draining'
      route.fulfill({ status: 200, json: { ...HOST_ACTIVE, operational_state: 'draining' } })
    })

    await page.goto('/admin/hosts')
    await page.getByTestId(`host-action-drain-${HOST_ACTIVE.id}`).click()
    await expect(page.getByTestId(`host-status-${HOST_ACTIVE.id}`)).toContainText('ドレイン中')
  })

  test('ホストのvCPUとメモリ使用量が正しく表示される', async ({ page }) => {
    await authInit(page)
    await page.route('**/api/v1/hosts', (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: { items: [HOST_ACTIVE], next_cursor: '' } })
      } else {
        route.continue()
      }
    })

    await page.goto('/admin/hosts')
    await expect(page.getByTestId(`host-row-${HOST_ACTIVE.id}`)).toBeVisible()
    // vCPU: used/total = 4 / 32
    const row = page.getByTestId(`host-row-${HOST_ACTIVE.id}`)
    await expect(row).toContainText('4 / 32')
    // Memory: used/total = 8192 MB → 8 GB used, 65536 MB → 64 GB total
    await expect(row).toContainText('8 / 64 GB')
  })

  test('retire 時に確認ダイアログが表示される', async ({ page }) => {
    await authInit(page)
    await page.route('**/api/v1/hosts', (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: { items: [HOST_MAINTENANCE], next_cursor: '' } })
      } else {
        route.continue()
      }
    })

    await page.goto('/admin/hosts')
    await page.getByTestId(`host-action-retire-${HOST_MAINTENANCE.id}`).click()
    await expect(page.getByTestId('confirm-retire-dialog')).toBeVisible()
  })

  test('ホスト作成フォームを開いて送信できる', async ({ page }) => {
    await authInit(page)

    let callCount = 0
    await page.route('**/api/v1/hosts', (route) => {
      if (route.request().method() === 'GET') {
        const items = callCount === 0 ? [] : [HOST_ACTIVE]
        callCount++
        route.fulfill({ status: 200, json: { items, next_cursor: '' } })
      } else if (route.request().method() === 'POST') {
        route.fulfill({ status: 201, json: HOST_ACTIVE })
      } else {
        route.continue()
      }
    })

    await page.goto('/admin/hosts')
    await page.getByTestId('create-host-button').click()
    await expect(page.getByTestId('create-host-dialog')).toBeVisible()

    await page.getByTestId('host-name-input').fill('host-01')
    await page.getByTestId('host-address-input').fill('192.168.1.10')
    await page.getByTestId('create-host-submit').click()

    await expect(page.getByTestId('create-host-dialog')).not.toBeVisible()
    await expect(page.getByTestId(`host-row-${HOST_ACTIVE.id}`)).toBeVisible()
  })
})

// ---------------------------------------------------------------------------
// S046-2: ストレージ管理
// ---------------------------------------------------------------------------

test.describe('S046-2: ストレージ管理', () => {
  function mockStorage(page: import('@playwright/test').Page, opts: { backends?: typeof BACKEND_1[], volumeTypes?: typeof VOLTYPE_1[] } = {}) {
    const backends = opts.backends ?? [BACKEND_1]
    const volumeTypes = opts.volumeTypes ?? [VOLTYPE_1]

    page.route('**/api/v1/storage-domains', (route) => {
      route.fulfill({ status: 200, json: [STORAGE_DOMAIN_1] })
    })
    page.route('**/api/v1/admin/storage-backends', (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: backends })
      } else if (route.request().method() === 'POST') {
        route.fulfill({ status: 201, json: BACKEND_1 })
      } else {
        route.continue()
      }
    })
    page.route('**/api/v1/admin/storage-backends/**', (route) => {
      if (route.request().method() === 'DELETE') {
        route.fulfill({ status: 204, body: '' })
      } else {
        route.continue()
      }
    })
    page.route('**/api/v1/volume-types', (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: volumeTypes })
      } else {
        route.continue()
      }
    })
    page.route('**/api/v1/admin/volume-types', (route) => {
      if (route.request().method() === 'POST') {
        route.fulfill({ status: 201, json: VOLTYPE_1 })
      } else {
        route.continue()
      }
    })
    page.route('**/api/v1/admin/volume-types/**', (route) => {
      if (route.request().method() === 'DELETE') {
        route.fulfill({ status: 204, body: '' })
      } else {
        route.continue()
      }
    })
  }

  test('ストレージ管理ページが表示される', async ({ page }) => {
    await authInit(page)
    await mockStorage(page)

    await page.goto('/admin/storage')
    await expect(page.getByTestId(`backend-row-${BACKEND_1.id}`)).toBeVisible()
    await expect(page.getByTestId(`volume-type-row-${VOLTYPE_1.id}`)).toBeVisible()
  })

  test('Storage Backend を削除するとリストから消える', async ({ page }) => {
    await authInit(page)
    let deleted = false
    await page.route('**/api/v1/storage-domains', (route) => {
      route.fulfill({ status: 200, json: [STORAGE_DOMAIN_1] })
    })
    await page.route('**/api/v1/admin/storage-backends', (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: deleted ? [] : [BACKEND_1] })
      } else {
        route.continue()
      }
    })
    await page.route(`**/api/v1/admin/storage-backends/${BACKEND_1.id}`, (route) => {
      if (route.request().method() === 'DELETE') {
        deleted = true
        route.fulfill({ status: 204, body: '' })
      } else {
        route.continue()
      }
    })
    await page.route('**/api/v1/volume-types', (route) => {
      route.fulfill({ status: 200, json: [] })
    })

    await page.goto('/admin/storage')
    await page.getByTestId(`delete-backend-button-${BACKEND_1.id}`).click()
    await expect(page.getByTestId('confirm-delete-dialog')).toBeVisible()
    await page.getByTestId('confirm-delete-button').click()

    await expect(page.getByTestId(`backend-row-${BACKEND_1.id}`)).not.toBeVisible()
  })
})

// ---------------------------------------------------------------------------
// S046-2: GW ノード / IP プール管理
// ---------------------------------------------------------------------------

test.describe('S046-2: ネットワーク管理 (GW ノード)', () => {
  function mockNetworkInfra(page: import('@playwright/test').Page, opts: { gwNodes?: typeof GW_NODE_1[], ipPools?: typeof IP_POOL_1[] } = {}) {
    const gwNodes = opts.gwNodes ?? []
    const ipPools = opts.ipPools ?? []

    page.route('**/api/v1/admin/gateway-nodes', (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: gwNodes })
      } else if (route.request().method() === 'POST') {
        route.fulfill({ status: 201, json: GW_NODE_1 })
      } else {
        route.continue()
      }
    })
    page.route('**/api/v1/admin/gateway-nodes/**', (route) => {
      if (route.request().method() === 'DELETE') {
        route.fulfill({ status: 204, body: '' })
      } else {
        route.continue()
      }
    })
    page.route('**/api/v1/admin/ip-pools', (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: ipPools })
      } else if (route.request().method() === 'POST') {
        route.fulfill({ status: 201, json: IP_POOL_1 })
      } else {
        route.continue()
      }
    })
    page.route('**/api/v1/admin/ip-pools/**', (route) => {
      if (route.request().method() === 'DELETE') {
        route.fulfill({ status: 204, body: '' })
      } else {
        route.continue()
      }
    })
  }

  test('GW ノードが空の場合は空状態が表示される', async ({ page }) => {
    await authInit(page)
    await mockNetworkInfra(page)

    await page.goto('/admin/network')
    await expect(page.getByTestId('empty-gateway-nodes')).toBeVisible()
    await expect(page.getByTestId('create-gateway-node-button')).toBeEnabled()
  })

  test('GW ノードを作成できる', async ({ page }) => {
    await authInit(page)

    let created = false
    await page.route('**/api/v1/admin/gateway-nodes', (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: created ? [GW_NODE_1] : [] })
      } else if (route.request().method() === 'POST') {
        created = true
        route.fulfill({ status: 201, json: GW_NODE_1 })
      } else {
        route.continue()
      }
    })
    await page.route('**/api/v1/admin/ip-pools', (route) => {
      route.fulfill({ status: 200, json: [] })
    })

    await page.goto('/admin/network')
    await page.getByTestId('create-gateway-node-button').click()
    await expect(page.getByTestId('create-gateway-node-dialog')).toBeVisible()

    await page.getByTestId('gateway-node-host-id-input').fill(HOST_ACTIVE.id)
    await page.getByTestId('gateway-node-external-ip-input').fill('203.0.113.1')
    await page.getByTestId('gateway-node-internal-ip-input').fill('10.0.0.1')
    await page.getByTestId('create-gateway-node-submit').click()

    await expect(page.getByTestId('create-gateway-node-dialog')).not.toBeVisible()
    await expect(page.getByTestId(`gateway-node-row-${GW_NODE_1.id}`)).toBeVisible()
  })

  test('GW ノードを削除できる', async ({ page }) => {
    await authInit(page)

    let deleted = false
    await page.route('**/api/v1/admin/gateway-nodes', (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: deleted ? [] : [GW_NODE_1] })
      } else {
        route.continue()
      }
    })
    await page.route(`**/api/v1/admin/gateway-nodes/${GW_NODE_1.id}`, (route) => {
      if (route.request().method() === 'DELETE') {
        deleted = true
        route.fulfill({ status: 204, body: '' })
      } else {
        route.continue()
      }
    })
    await page.route('**/api/v1/admin/ip-pools', (route) => {
      route.fulfill({ status: 200, json: [] })
    })

    await page.goto('/admin/network')
    await page.getByTestId(`delete-gateway-node-button-${GW_NODE_1.id}`).click()
    await expect(page.getByTestId('confirm-delete-dialog')).toBeVisible()
    await page.getByTestId('confirm-delete-button').click()

    await expect(page.getByTestId(`gateway-node-row-${GW_NODE_1.id}`)).not.toBeVisible()
  })

  test('IP プールを作成できる', async ({ page }) => {
    await authInit(page)

    let created = false
    await page.route('**/api/v1/admin/gateway-nodes', (route) => {
      route.fulfill({ status: 200, json: [] })
    })
    await page.route('**/api/v1/admin/ip-pools', (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: created ? [IP_POOL_1] : [] })
      } else if (route.request().method() === 'POST') {
        created = true
        route.fulfill({ status: 201, json: IP_POOL_1 })
      } else {
        route.continue()
      }
    })

    await page.goto('/admin/network')
    await page.getByTestId('create-ip-pool-button').click()
    await expect(page.getByTestId('create-ip-pool-dialog')).toBeVisible()

    await page.getByTestId('ip-pool-name-input').fill('public-pool')
    await page.getByTestId('ip-pool-cidr-input').fill('203.0.113.0/24')
    await page.getByTestId('create-ip-pool-submit').click()

    await expect(page.getByTestId('create-ip-pool-dialog')).not.toBeVisible()
    await expect(page.getByTestId(`ip-pool-row-${IP_POOL_1.id}`)).toBeVisible()
  })
})
