import { test, expect } from '@playwright/test'

const NET_ID = 'net-0000-0000-0000-000000000001'
const GROUP_A_ID = 'grp-0000-0000-0000-000000000001'
const GROUP_B_ID = 'grp-0000-0000-0000-000000000002'
const POLICY_ID = 'pol-0000-0000-0000-000000000001'
const TENANT_1 = { id: 'test-tenant-id', name: 'Test Tenant', organization_id: 'org-1', created_at: '2024-01-01T00:00:00Z' }

const NET_1 = {
  id: NET_ID,
  tenant_id: TENANT_1.id,
  name: 'my-network',
  cidr: '10.0.0.0/24',
  vni: 1001,
  status: 'active',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
}

const GROUP_A = { id: GROUP_A_ID, network_id: NET_ID, name: 'web', created_at: '2024-01-01T00:00:00Z' }
const GROUP_B = { id: GROUP_B_ID, network_id: NET_ID, name: 'api', created_at: '2024-01-01T00:00:00Z' }

const POLICY_1 = {
  id: POLICY_ID,
  network_id: NET_ID,
  src_group_id: GROUP_A_ID,
  dst_group_id: GROUP_B_ID,
  protocol: 'tcp',
  dst_port: 8080,
  priority: 1000,
  action: 'allow',
  created_at: '2024-01-01T00:00:00Z',
}

test.describe('S048-1: ネットワーク管理', () => {
  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('cirrus_token', 'test-token')
      localStorage.setItem('cirrus_tenant_id', 'test-tenant-id')
    })
    await page.route('**/api/v1/me/tenants', (route) =>
      route.fulfill({ status: 200, json: { items: [TENANT_1], next_cursor: '' } })
    )
    await page.route('**/api/v1/me', (route) =>
      route.fulfill({ status: 200, json: { id: 'user-1', email: 'test@example.com' } })
    )
  })

  test('ネットワーク一覧: ネットワークが存在しない場合に空状態を表示する', async ({ page }) => {
    await page.route('**/api/v1/networks', (route) =>
      route.fulfill({ status: 200, json: { items: [], next_cursor: '' } })
    )
    await page.goto('/networks')
    await expect(page.getByTestId('network-empty-state')).toBeVisible()
    await expect(page.getByTestId('create-network-button')).toBeEnabled()
  })

  test('ネットワーク作成: 名前とCIDRを入力して作成できる', async ({ page }) => {
    let networkCreated = false
    await page.route('**/api/v1/networks', async (route) => {
      if (route.request().method() === 'GET') {
        const items = networkCreated ? [NET_1] : []
        return route.fulfill({ status: 200, json: { items, next_cursor: '' } })
      }
      if (route.request().method() === 'POST') {
        const body = JSON.parse(route.request().postData() ?? '{}')
        expect(body.name).toBeTruthy()
        networkCreated = true
        return route.fulfill({ status: 201, json: NET_1 })
      }
    })

    await page.goto('/networks')
    await expect(page.getByTestId('network-empty-state')).toBeVisible()

    await page.getByTestId('create-network-button').click()
    await expect(page.getByTestId('network-create-dialog')).toBeVisible()

    await page.getByTestId('network-create-name').fill('my-network')
    await page.getByTestId('network-create-cidr').fill('10.0.0.0/24')
    await page.getByTestId('network-create-submit').click()

    await expect(page.getByTestId('network-create-dialog')).not.toBeVisible()
    await expect(page.getByTestId(`network-row-${NET_ID}`)).toBeVisible()
    await expect(page.getByTestId(`network-row-${NET_ID}`)).toContainText('my-network')
    await expect(page.getByTestId(`network-row-${NET_ID}`)).toContainText('10.0.0.0/24')
  })

  test('ネットワーク作成: CIDRを省略すると自動割り当てで作成できる', async ({ page }) => {
    const autoNet = { ...NET_1, name: 'auto-network', cidr: '100.64.0.0/24' }
    await page.route('**/api/v1/networks', async (route) => {
      if (route.request().method() === 'GET') {
        return route.fulfill({ status: 200, json: { items: [autoNet], next_cursor: '' } })
      }
      if (route.request().method() === 'POST') {
        const body = JSON.parse(route.request().postData() ?? '{}')
        expect(body.cidr).toBeUndefined()
        return route.fulfill({ status: 201, json: autoNet })
      }
    })

    await page.goto('/networks')
    await page.getByTestId('create-network-button').click()
    await page.getByTestId('network-create-name').fill('auto-network')
    // CIDRを空のまま送信
    await page.getByTestId('network-create-submit').click()
    await expect(page.getByTestId('network-create-dialog')).not.toBeVisible()
    await expect(page.getByTestId(`network-row-${NET_ID}`).first()).toBeVisible()
  })

  test('ネットワーク削除: 確認後に削除される', async ({ page }) => {
    let deleted = false
    await page.route('**/api/v1/networks', (route) => {
      const items = deleted ? [] : [NET_1]
      route.fulfill({ status: 200, json: { items, next_cursor: '' } })
    })
    await page.route(`**/api/v1/networks/${NET_ID}`, async (route) => {
      if (route.request().method() === 'DELETE') {
        deleted = true
        return route.fulfill({ status: 204, body: '' })
      }
    })

    await page.goto('/networks')
    await expect(page.getByTestId(`network-row-${NET_ID}`)).toBeVisible()

    await page.getByTestId(`network-delete-${NET_ID}`).click()
    await expect(page.getByTestId('network-delete-dialog')).toBeVisible()
    await page.getByTestId('network-delete-confirm').click()

    await expect(page.getByTestId('network-empty-state')).toBeVisible()
  })

  test('グループ管理: ネットワーク行を展開してグループを作成・削除できる', async ({ page }) => {
    let groups: typeof GROUP_A[] = []

    await page.route('**/api/v1/networks', (route) =>
      route.fulfill({ status: 200, json: { items: [NET_1], next_cursor: '' } })
    )
    await page.route(`**/api/v1/networks/${NET_ID}/groups`, async (route) => {
      if (route.request().method() === 'GET') {
        return route.fulfill({ status: 200, json: { items: groups, next_cursor: '' } })
      }
      if (route.request().method() === 'POST') {
        const body = JSON.parse(route.request().postData() ?? '{}')
        expect(body.name).toBeTruthy()
        groups = [GROUP_A]
        return route.fulfill({ status: 201, json: GROUP_A })
      }
    })
    await page.route(`**/api/v1/networks/${NET_ID}/groups/${GROUP_A_ID}`, async (route) => {
      if (route.request().method() === 'DELETE') {
        groups = []
        return route.fulfill({ status: 204, body: '' })
      }
    })
    // policies endpoint needed when expanding
    await page.route(`**/api/v1/networks/${NET_ID}/policies`, (route) =>
      route.fulfill({ status: 200, json: { items: [], next_cursor: '' } })
    )

    await page.goto('/networks')
    await page.getByTestId(`network-expand-${NET_ID}`).click()
    await expect(page.getByTestId(`groups-panel-${NET_ID}`)).toBeVisible()
    await expect(page.getByTestId(`group-empty-${NET_ID}`)).toBeVisible()

    // グループ追加
    await page.getByTestId(`add-group-button-${NET_ID}`).click()
    await page.getByTestId(`group-name-input-${NET_ID}`).fill('web')
    await page.getByTestId(`group-create-submit-${NET_ID}`).click()
    await expect(page.getByTestId(`group-item-${GROUP_A_ID}`)).toBeVisible()
    await expect(page.getByTestId(`group-item-${GROUP_A_ID}`)).toContainText('web')

    // グループ削除
    await page.getByTestId(`group-delete-${GROUP_A_ID}`).click()
    await expect(page.getByTestId('group-delete-dialog')).toBeVisible()
    await page.getByTestId('group-delete-confirm').click()
    await expect(page.getByTestId(`group-item-${GROUP_A_ID}`)).not.toBeVisible()
  })

  test('ポリシー管理: src/dstグループを選択してポリシーを作成・削除できる', async ({ page }) => {
    let policies: typeof POLICY_1[] = []

    await page.route('**/api/v1/networks', (route) =>
      route.fulfill({ status: 200, json: { items: [NET_1], next_cursor: '' } })
    )
    await page.route(`**/api/v1/networks/${NET_ID}/groups`, (route) =>
      route.fulfill({ status: 200, json: { items: [GROUP_A, GROUP_B], next_cursor: '' } })
    )
    await page.route(`**/api/v1/networks/${NET_ID}/policies`, async (route) => {
      if (route.request().method() === 'GET') {
        return route.fulfill({ status: 200, json: { items: policies, next_cursor: '' } })
      }
      if (route.request().method() === 'POST') {
        const body = JSON.parse(route.request().postData() ?? '{}')
        expect(body.src_group_id).toBe(GROUP_A_ID)
        expect(body.dst_group_id).toBe(GROUP_B_ID)
        expect(body.protocol).toBe('tcp')
        policies = [POLICY_1]
        return route.fulfill({ status: 201, json: POLICY_1 })
      }
    })
    await page.route(`**/api/v1/networks/${NET_ID}/policies/${POLICY_ID}`, async (route) => {
      if (route.request().method() === 'DELETE') {
        policies = []
        return route.fulfill({ status: 204, body: '' })
      }
    })

    await page.goto('/networks')
    await page.getByTestId(`network-expand-${NET_ID}`).click()
    await expect(page.getByTestId(`policies-panel-${NET_ID}`)).toBeVisible()

    // ポリシー追加フォームを開く
    await page.getByTestId(`add-policy-button-${NET_ID}`).click()
    await expect(page.getByTestId(`policy-form-${NET_ID}`)).toBeVisible()

    // src/dst グループ選択
    await page.getByTestId(`policy-src-group-${NET_ID}`).selectOption(GROUP_A_ID)
    await page.getByTestId(`policy-dst-group-${NET_ID}`).selectOption(GROUP_B_ID)
    await page.getByTestId(`policy-protocol-${NET_ID}`).selectOption('tcp')
    await page.getByTestId(`policy-dst-port-${NET_ID}`).fill('8080')
    await page.getByTestId(`policy-action-${NET_ID}`).selectOption('allow')
    await page.getByTestId(`policy-create-submit-${NET_ID}`).click()

    await expect(page.getByTestId(`policy-item-${POLICY_ID}`)).toBeVisible()
    await expect(page.getByTestId(`policy-item-${POLICY_ID}`)).toContainText('tcp')

    // ポリシー削除
    await page.getByTestId(`policy-delete-${POLICY_ID}`).click()
    await expect(page.getByTestId('policy-delete-dialog')).toBeVisible()
    await page.getByTestId('policy-delete-confirm').click()
    await expect(page.getByTestId(`policy-item-${POLICY_ID}`)).not.toBeVisible()
  })

  test('ネットワーク一覧: API失敗時にエラーメッセージを表示する', async ({ page }) => {
    await page.route('**/api/v1/networks', (route) =>
      route.fulfill({ status: 500, json: { error: 'internal server error' } })
    )
    await page.goto('/networks')
    await expect(page.getByTestId('error-message')).toBeVisible()
  })
})
