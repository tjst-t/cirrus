import { test, expect } from '@playwright/test'

const VM_RUNNING = {
  id: 'vm-1111-0000-0000-0000-000000000001',
  name: 'my-vm',
  status: 'running' as const,
  flavor_id: 'fl-1111',
  network_id: 'net-1111',
  ip_address: '10.0.0.5',
  host_id: 'host-1111',
  created_at: '2024-01-01T00:00:00Z',
}

const VM_ERROR = {
  ...VM_RUNNING,
  id: 'vm-2222-0000-0000-0000-000000000002',
  name: 'broken-vm',
  status: 'error' as const,
  error_message: 'no suitable host available',
}

const VM_STOPPED = {
  ...VM_RUNNING,
  id: 'vm-3333-0000-0000-0000-000000000003',
  name: 'stopped-vm',
  status: 'stopped' as const,
}

const FLAVOR_1 = { id: 'fl-1111', name: 'm1.small', vcpus: 1, ram_mb: 1024, disk_gb: 20 }
const TENANT_1 = { id: 'test-tenant-id', name: 'Test Tenant', organization_id: 'org-1', created_at: '2024-01-01T00:00:00Z' }

function authInit(page: import('@playwright/test').Page) {
  return page.addInitScript(() => {
    localStorage.setItem('cirrus_token', 'test-token')
    localStorage.setItem('cirrus_tenant_id', 'test-tenant-id')
  })
}

// mockCommonRoutes mocks routes that TenantLayout and VmDetailPage always call.
async function mockCommonRoutes(page: import('@playwright/test').Page, vm: typeof VM_RUNNING, deleteSucceeds = false) {
  await page.route('**/api/v1/me/tenants', (route) => {
    route.fulfill({ status: 200, json: { items: [TENANT_1], next_cursor: '' } })
  })
  await page.route('**/api/v1/flavors', (route) => {
    route.fulfill({ status: 200, json: { items: [FLAVOR_1], next_cursor: '' } })
  })
  await page.route(`**/api/v1/vms/${vm.id}`, (route) => {
    if (route.request().method() === 'GET') {
      route.fulfill({ status: 200, json: vm })
    } else if (route.request().method() === 'DELETE' && deleteSucceeds) {
      route.fulfill({ status: 204, body: '' })
    } else {
      route.continue()
    }
  })
}

test.describe('VM 詳細ページ', () => {
  test('VM 詳細ページが表示される', async ({ page }) => {
    await authInit(page)
    await mockCommonRoutes(page, VM_RUNNING)

    await page.goto(`/vms/${VM_RUNNING.id}`)
    await expect(page.getByText('my-vm').first()).toBeVisible()
  })

  test('Flavor 情報が表示される', async ({ page }) => {
    await authInit(page)
    await mockCommonRoutes(page, VM_RUNNING)

    await page.goto(`/vms/${VM_RUNNING.id}`)
    await expect(page.getByTestId('vm-flavor-name')).toHaveText('m1.small')
    await expect(page.getByTestId('vm-flavor-vcpus')).toHaveText('1 コア')
  })

  test('エラーメッセージが表示される', async ({ page }) => {
    await authInit(page)
    await mockCommonRoutes(page, VM_ERROR)

    await page.goto(`/vms/${VM_ERROR.id}`)
    await expect(page.getByTestId('vm-error-message')).toHaveText('no suitable host available')
  })

  test('running VM では起動ボタンが無効で停止ボタンが有効', async ({ page }) => {
    await authInit(page)
    await mockCommonRoutes(page, VM_RUNNING)

    await page.goto(`/vms/${VM_RUNNING.id}`)
    await expect(page.getByRole('button', { name: '起動', exact: true })).toBeDisabled()
    await expect(page.getByRole('button', { name: '停止', exact: true })).toBeEnabled()
  })

  test('stopped VM では起動ボタンが有効で停止ボタンが無効', async ({ page }) => {
    await authInit(page)
    await mockCommonRoutes(page, VM_STOPPED)

    await page.goto(`/vms/${VM_STOPPED.id}`)
    await expect(page.getByRole('button', { name: '起動', exact: true })).toBeEnabled()
    await expect(page.getByRole('button', { name: '停止', exact: true })).toBeDisabled()
  })

  test('削除ボタンで確認ダイアログが表示される', async ({ page }) => {
    await authInit(page)
    await mockCommonRoutes(page, VM_RUNNING)

    await page.goto(`/vms/${VM_RUNNING.id}`)
    await page.getByTestId('vm-delete-button').click()
    await expect(page.getByTestId('vm-delete-confirm-dialog')).toBeVisible()
  })

  test('削除確定後に VM 一覧ページへ遷移する', async ({ page }) => {
    await authInit(page)
    await mockCommonRoutes(page, VM_RUNNING, true)
    await page.route('**/api/v1/vms', (route) => {
      route.fulfill({ status: 200, json: { items: [], next_cursor: '' } })
    })

    await page.goto(`/vms/${VM_RUNNING.id}`)
    await page.getByTestId('vm-delete-button').click()
    await page.getByTestId('vm-delete-confirm-button').click()

    await expect(page).toHaveURL('/vms')
  })

  test('「← VM 一覧」リンクで一覧ページへ戻れる', async ({ page }) => {
    await authInit(page)
    await mockCommonRoutes(page, VM_RUNNING)
    await page.route('**/api/v1/vms', (route) => {
      route.fulfill({ status: 200, json: { items: [], next_cursor: '' } })
    })

    await page.goto(`/vms/${VM_RUNNING.id}`)
    await page.getByText('← VM 一覧').click()
    await expect(page).toHaveURL('/vms')
  })
})
