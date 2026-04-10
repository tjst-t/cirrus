import { test, expect } from '@playwright/test'

const FLAVOR_1 = { id: 'fl-1111', name: 'm1.small', vcpus: 1, ram_mb: 1024, disk_gb: 20, created_at: '2024-01-01T00:00:00Z' }
const FLAVOR_2 = { id: 'fl-2222', name: 'm1.medium', vcpus: 2, ram_mb: 2048, disk_gb: 40, created_at: '2024-01-01T00:00:00Z' }

function authInit(page: import('@playwright/test').Page) {
  return page.addInitScript(() => {
    localStorage.setItem('cirrus_token', 'test-token')
    localStorage.setItem('cirrus_tenant_id', 'test-tenant-id')
  })
}

function mockFlavors(page: import('@playwright/test').Page, flavors: typeof FLAVOR_1[]) {
  page.route('**/api/v1/flavors', (route) => {
    if (route.request().method() === 'GET') {
      route.fulfill({ status: 200, json: { items: flavors, next_cursor: '' } })
    } else {
      route.continue()
    }
  })
  page.route('**/api/v1/admin/flavors', (route) => {
    if (route.request().method() === 'POST') {
      route.fulfill({ status: 201, json: FLAVOR_1 })
    } else {
      route.continue()
    }
  })
  page.route('**/api/v1/admin/flavors/**', (route) => {
    if (route.request().method() === 'DELETE') {
      route.fulfill({ status: 204, body: '' })
    } else {
      route.continue()
    }
  })
}

test.describe('コンピュート管理 (Flavor CRUD)', () => {
  test('Flavor 一覧が表示される', async ({ page }) => {
    await authInit(page)
    await mockFlavors(page, [FLAVOR_1, FLAVOR_2])

    await page.goto('/admin/compute')
    await expect(page.getByTestId(`flavor-row-${FLAVOR_1.id}`)).toBeVisible()
    await expect(page.getByTestId(`flavor-row-${FLAVOR_2.id}`)).toBeVisible()
  })

  test('Flavor がない場合は空メッセージが表示される', async ({ page }) => {
    await authInit(page)
    await mockFlavors(page, [])

    await page.goto('/admin/compute')
    await expect(page.getByText('Flavor なし')).toBeVisible()
  })

  test('Flavor を作成できる', async ({ page }) => {
    await authInit(page)

    let created = false
    await page.route('**/api/v1/flavors', (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: { items: created ? [FLAVOR_1] : [], next_cursor: '' } })
      } else {
        route.continue()
      }
    })
    await page.route('**/api/v1/admin/flavors', (route) => {
      if (route.request().method() === 'POST') {
        created = true
        route.fulfill({ status: 201, json: FLAVOR_1 })
      } else {
        route.continue()
      }
    })

    await page.goto('/admin/compute')
    await page.getByTestId('create-flavor-button').click()
    await expect(page.getByTestId('create-flavor-dialog')).toBeVisible()

    await page.getByTestId('flavor-name-input').fill('m1.small')
    await page.getByTestId('flavor-vcpus-input').fill('1')
    await page.getByTestId('flavor-memory-input').fill('1024')
    await page.getByTestId('flavor-disk-input').fill('20')
    await page.getByTestId('create-flavor-submit').click()

    await expect(page.getByTestId('create-flavor-dialog')).not.toBeVisible()
    await expect(page.getByTestId(`flavor-row-${FLAVOR_1.id}`)).toBeVisible()
  })

  test('Flavor を削除できる', async ({ page }) => {
    await authInit(page)

    let deleted = false
    await page.route('**/api/v1/flavors', (route) => {
      if (route.request().method() === 'GET') {
        route.fulfill({ status: 200, json: { items: deleted ? [] : [FLAVOR_1], next_cursor: '' } })
      } else {
        route.continue()
      }
    })
    await page.route(`**/api/v1/admin/flavors/${FLAVOR_1.id}`, (route) => {
      if (route.request().method() === 'DELETE') {
        deleted = true
        route.fulfill({ status: 204, body: '' })
      } else {
        route.continue()
      }
    })

    await page.goto('/admin/compute')
    await page.getByTestId(`delete-flavor-button-${FLAVOR_1.id}`).click()
    await expect(page.getByTestId('confirm-delete-dialog')).toBeVisible()
    await page.getByTestId('confirm-delete-button').click()

    await expect(page.getByTestId(`flavor-row-${FLAVOR_1.id}`)).not.toBeVisible()
  })

  test('Flavor テーブルに vCPU / メモリ / ディスクが表示される', async ({ page }) => {
    await authInit(page)
    await mockFlavors(page, [FLAVOR_1])

    await page.goto('/admin/compute')
    const row = page.getByTestId(`flavor-row-${FLAVOR_1.id}`)
    await expect(row).toContainText('m1.small')
    await expect(row).toContainText('1')    // vcpus
    await expect(row).toContainText('1.0')  // ram_mb / 1024 = 1.0 GB
    await expect(row).toContainText('20')   // disk_gb
  })
})
