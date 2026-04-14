import { test, expect } from '@playwright/test'

// ORG_1 / TENANT_1 fixtures
const ORG_1 = {
  id: 'org-aaaaaaaa-0000-0000-0000-000000000001',
  name: 'test-org',
  created_at: '2024-01-01T00:00:00Z',
}
const TENANT_1 = {
  id: 'ten-bbbbbbbb-0000-0000-0000-000000000001',
  name: 'test-tenant',
  organization_id: ORG_1.id,
  created_at: '2024-01-01T00:00:00Z',
}
const ROLE_1 = {
  user_id: 'usr-cccccccc-0000-0000-0000-000000000001',
  role: 'admin',
  tenant_id: TENANT_1.id,
}

function authInit(page: import('@playwright/test').Page) {
  return page.addInitScript(() => {
    localStorage.setItem('cirrus_token', 'test-token')
    localStorage.setItem('cirrus_tenant_id', 'test-tenant-id')
  })
}

// ---------------------------------------------------------------------------
// S046-1: 組織一覧
// ---------------------------------------------------------------------------

test.describe('S046-1: 組織一覧', () => {
  test('空の状態が表示される', async ({ page }) => {
    await authInit(page)
    const mockOrgs = await page.route('**/api/v1/organizations', (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: { items: [], next_cursor: '' } })
      } else {
        route.continue()
      }
    })

    await page.goto('/admin/organizations')
    await expect(page.getByTestId('empty-orgs')).toBeVisible()
    await expect(page.getByTestId('create-org-button')).toBeEnabled()
    void mockOrgs
  })

  test('組織リストが表示される', async ({ page }) => {
    await authInit(page)
    await page.route('**/api/v1/organizations', (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: { items: [ORG_1], next_cursor: '' } })
      } else {
        route.continue()
      }
    })

    await page.goto('/admin/organizations')
    await expect(page.getByTestId(`org-row-${ORG_1.id}`)).toBeVisible()
    await expect(page.getByTestId(`org-row-${ORG_1.id}`)).toContainText(ORG_1.name)
  })

  test('API 失敗時にエラーが表示される', async ({ page }) => {
    await authInit(page)
    await page.route('**/api/v1/organizations', (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 500, json: { error: 'internal server error' } })
      } else {
        route.continue()
      }
    })

    await page.goto('/admin/organizations')
    await expect(page.getByTestId('org-list-error')).toBeVisible()
  })
})

// ---------------------------------------------------------------------------
// S046-1: 組織作成
// ---------------------------------------------------------------------------

test.describe('S046-1: 組織作成', () => {
  test('組織を作成するとリストに追加される', async ({ page }) => {
    await authInit(page)

    let callCount = 0
    await page.route('**/api/v1/organizations', (route) => {
      if (route.request().method() === 'GET') {
        const items = callCount === 0 ? [] : [ORG_1]
        callCount++
        route.fulfill({ status: 200, json: { items, next_cursor: '' } })
      } else if (route.request().method() === 'POST') {
        const body = route.request().postDataJSON()
        expect(body.name).toBeTruthy()
        route.fulfill({ status: 201, json: ORG_1 })
      } else {
        route.continue()
      }
    })

    await page.goto('/admin/organizations')
    await page.getByTestId('create-org-button').click()
    await expect(page.getByTestId('create-org-dialog')).toBeVisible()

    await page.getByTestId('org-name-input').fill('test-org')
    await page.getByTestId('create-org-submit').click()

    await expect(page.getByTestId('create-org-dialog')).not.toBeVisible()
    await expect(page.getByTestId(`org-row-${ORG_1.id}`)).toBeVisible()
  })

  test('名前が空の場合は送信できない', async ({ page }) => {
    await authInit(page)
    await page.route('**/api/v1/organizations', (route) => {
      route.fulfill({ status: 200, json: { items: [], next_cursor: '' } })
    })

    await page.goto('/admin/organizations')
    await page.getByTestId('create-org-button').click()
    // 入力なしで送信 → ダイアログが閉じないこと
    await page.getByTestId('create-org-submit').click()
    await expect(page.getByTestId('create-org-dialog')).toBeVisible()
  })

  test('作成 API 失敗時にダイアログ内にエラーが表示される', async ({ page }) => {
    await authInit(page)
    await page.route('**/api/v1/organizations', (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: { items: [], next_cursor: '' } })
      } else if (route.request().method() === 'POST') {
        route.fulfill({ status: 409, json: { error: 'organization name already exists' } })
      } else {
        route.continue()
      }
    })

    await page.goto('/admin/organizations')
    await page.getByTestId('create-org-button').click()
    await page.getByTestId('org-name-input').fill('dup-org')
    await page.getByTestId('create-org-submit').click()
    await expect(page.getByTestId('create-org-dialog')).toBeVisible()
    await expect(page.getByTestId('create-org-error')).toBeVisible()
  })
})

// ---------------------------------------------------------------------------
// S046-1: テナント作成
// ---------------------------------------------------------------------------

test.describe('S046-1: テナント作成', () => {
  test('組織を展開してテナントを作成できる', async ({ page }) => {
    await authInit(page)

    let tenantCallCount = 0
    await page.route('**/api/v1/organizations', (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: { items: [ORG_1], next_cursor: '' } })
      } else {
        route.continue()
      }
    })
    await page.route(`**/api/v1/organizations/${ORG_1.id}/tenants`, (route) => {
      if (route.request().method() === 'GET') {
        const items = tenantCallCount === 0 ? [] : [TENANT_1]
        tenantCallCount++
        route.fulfill({ status: 200, json: { items, next_cursor: '' } })
      } else if (route.request().method() === 'POST') {
        const body = route.request().postDataJSON()
        expect(body.name).toBeTruthy()
        route.fulfill({ status: 201, json: TENANT_1 })
      } else {
        route.continue()
      }
    })

    await page.goto('/admin/organizations')
    await page.getByTestId(`expand-tenants-button-${ORG_1.id}`).click()
    await page.getByTestId(`create-tenant-button-${ORG_1.id}`).click()
    await expect(page.getByTestId('create-tenant-dialog')).toBeVisible()

    await page.getByTestId('tenant-name-input').fill('test-tenant')
    await page.getByTestId('create-tenant-submit').click()

    await expect(page.getByTestId('create-tenant-dialog')).not.toBeVisible()
    await expect(page.getByTestId(`tenant-row-${TENANT_1.id}`)).toBeVisible()
  })
})

// ---------------------------------------------------------------------------
// S046-1: ロール割り当て
// ---------------------------------------------------------------------------

test.describe('S046-1: ロール割り当て', () => {
  test('ロールを追加できる', async ({ page }) => {
    await authInit(page)

    let roleCallCount = 0
    await page.route('**/api/v1/organizations', (route) => {
      route.fulfill({ status: 200, json: { items: [ORG_1], next_cursor: '' } })
    })
    await page.route(`**/api/v1/organizations/${ORG_1.id}/tenants`, (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: { items: [TENANT_1], next_cursor: '' } })
      } else {
        route.continue()
      }
    })
    await page.route(`**/api/v1/tenants/${TENANT_1.id}/role-assignments`, (route) => {
      if (route.request().method() === 'GET') {
        const items = roleCallCount === 0 ? [] : [ROLE_1]
        roleCallCount++
        route.fulfill({ status: 200, json: items })
      } else if (route.request().method() === 'POST') {
        const body = route.request().postDataJSON()
        expect(body.user_id).toBeTruthy()
        expect(body.role).toBeTruthy()
        route.fulfill({ status: 201, json: ROLE_1 })
      } else {
        route.continue()
      }
    })

    await page.goto('/admin/organizations')
    await page.getByTestId(`expand-tenants-button-${ORG_1.id}`).click()
    await page.getByTestId(`role-management-button-${TENANT_1.id}`).click()
    await page.getByTestId('add-role-button').click()
    await expect(page.getByTestId('add-role-dialog')).toBeVisible()

    await page.getByTestId('role-user-id-input').fill(ROLE_1.user_id)
    await page.getByTestId('role-select').selectOption('admin')
    await page.getByTestId('add-role-submit').click()

    await expect(page.getByTestId('add-role-dialog')).not.toBeVisible()
    await expect(page.getByTestId(`role-row-${ROLE_1.user_id}`)).toBeVisible()
  })

  test('ロールを削除できる（確認ダイアログ）', async ({ page }) => {
    await authInit(page)

    await page.route('**/api/v1/organizations', (route) => {
      route.fulfill({ status: 200, json: { items: [ORG_1], next_cursor: '' } })
    })
    await page.route(`**/api/v1/organizations/${ORG_1.id}/tenants`, (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: { items: [TENANT_1], next_cursor: '' } })
      } else {
        route.continue()
      }
    })
    let deleted = false
    await page.route(`**/api/v1/tenants/${TENANT_1.id}/role-assignments`, (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: deleted ? [] : [ROLE_1] })
      } else {
        route.continue()
      }
    })
    await page.route(`**/api/v1/tenants/${TENANT_1.id}/role-assignments/${ROLE_1.user_id}`, (route) => {
      if (route.request().method() === 'DELETE') {
        deleted = true
        route.fulfill({ status: 204, body: '' })
      } else {
        route.continue()
      }
    })

    await page.goto('/admin/organizations')
    await page.getByTestId(`expand-tenants-button-${ORG_1.id}`).click()
    await page.getByTestId(`role-management-button-${TENANT_1.id}`).click()
    await page.getByTestId(`delete-role-button-${ROLE_1.user_id}`).click()
    await expect(page.getByTestId('confirm-delete-dialog')).toBeVisible()
    await page.getByTestId('confirm-delete-button').click()

    await expect(page.getByTestId(`role-row-${ROLE_1.user_id}`)).not.toBeVisible()
  })

  test('ユーザーID が空の場合はロール追加できない', async ({ page }) => {
    await authInit(page)

    await page.route('**/api/v1/organizations', (route) => {
      route.fulfill({ status: 200, json: { items: [ORG_1], next_cursor: '' } })
    })
    await page.route(`**/api/v1/organizations/${ORG_1.id}/tenants`, (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: { items: [TENANT_1], next_cursor: '' } })
      } else {
        route.continue()
      }
    })
    await page.route(`**/api/v1/tenants/${TENANT_1.id}/role-assignments`, (route) => {
      route.fulfill({ status: 200, json: [] })
    })

    await page.goto('/admin/organizations')
    await page.getByTestId(`expand-tenants-button-${ORG_1.id}`).click()
    await page.getByTestId(`role-management-button-${TENANT_1.id}`).click()
    await page.getByTestId('add-role-button').click()
    // ユーザーID 未入力で送信
    await page.getByTestId('add-role-submit').click()
    await expect(page.getByTestId('add-role-dialog')).toBeVisible()
  })
})
