import { test, expect } from '@playwright/test'

// VM states for the lifecycle test
let vmStatus: 'pending' | 'stopped' | 'running' = 'pending'

const VM_ID = 'vm-lifecycle-0000-0000-000000000001'
const FLAVOR_1 = { id: 'fl-1111', name: 'm1.small', vcpus: 1, ram_mb: 1024, disk_gb: 20 }
const NET_1 = { id: 'net-1111', name: 'default', cidr: '10.0.0.0/24', status: 'active', created_at: '2024-01-01T00:00:00Z' }
const AZ_1 = { id: 'az-1111', name: 'zone-a', description: 'Zone A', enabled: true, created_at: '2024-01-01T00:00:00Z', updated_at: '2024-01-01T00:00:00Z' }
const TENANT_1 = { id: 'test-tenant-id', name: 'Test Tenant', organization_id: 'org-1', created_at: '2024-01-01T00:00:00Z' }

function makeVm(status: 'pending' | 'stopped' | 'running') {
  return {
    id: VM_ID,
    name: 'lifecycle-vm',
    status,
    flavor_id: FLAVOR_1.id,
    network_id: NET_1.id,
    az_id: AZ_1.id,
    ip_address: status === 'running' ? '10.0.0.100' : undefined,
    created_at: '2024-01-01T00:00:00Z',
  }
}

test.describe('VM ライフサイクルフロー', () => {
  test.beforeEach(async ({ page }) => {
    // Reset state
    vmStatus = 'pending'

    await page.addInitScript(() => {
      localStorage.setItem('cirrus_token', 'test-token')
      localStorage.setItem('cirrus_tenant_id', 'test-tenant-id')
    })

    // Mock common tenant routes
    await page.route('**/api/v1/me/tenants', (route) => {
      route.fulfill({ status: 200, json: { items: [TENANT_1], next_cursor: '' } })
    })
    await page.route('**/api/v1/me', (route) => {
      route.fulfill({ status: 200, json: { id: 'user-1', email: 'test@example.com' } })
    })
  })

  test('VM を作成して起動・停止・削除できる', async ({ page }) => {
    // Track created VM
    let vmCreated = false
    let vmDeleted = false

    // Mock all API routes
    await page.route('**/api/v1/**', async (route) => {
      const url = route.request().url()
      const method = route.request().method()

      // VM list
      if (url.match(/\/api\/v1\/vms$/) && method === 'GET') {
        const items = vmCreated && !vmDeleted ? [makeVm(vmStatus)] : []
        return route.fulfill({ status: 200, json: { items, next_cursor: '' } })
      }

      // VM create
      if (url.match(/\/api\/v1\/vms$/) && method === 'POST') {
        const body = JSON.parse(route.request().postData() ?? '{}')
        expect(body.name).toBeTruthy()
        expect(body.flavor_id).toBeTruthy()
        vmCreated = true
        vmStatus = 'stopped'
        return route.fulfill({ status: 201, json: makeVm('stopped') })
      }

      // VM get by id
      if (url.match(/\/api\/v1\/vms\//) && method === 'GET' && !url.includes('/actions')) {
        if (!vmCreated || vmDeleted) {
          return route.fulfill({ status: 404, json: { error: 'not found' } })
        }
        return route.fulfill({ status: 200, json: makeVm(vmStatus) })
      }

      // VM action
      if (url.match(/\/api\/v1\/vms\/.*\/actions/) && method === 'POST') {
        const body = JSON.parse(route.request().postData() ?? '{}')
        if (body.action === 'start') vmStatus = 'running'
        if (body.action === 'stop') vmStatus = 'stopped'
        return route.fulfill({ status: 200, json: {} })
      }

      // VM delete
      if (url.match(/\/api\/v1\/vms\//) && method === 'DELETE') {
        vmDeleted = true
        return route.fulfill({ status: 204, body: '' })
      }

      // Flavors
      if (url.includes('/flavors')) {
        return route.fulfill({ status: 200, json: { items: [FLAVOR_1], next_cursor: '' } })
      }

      // Networks
      if (url.match(/\/networks$/) && method === 'GET') {
        return route.fulfill({ status: 200, json: { items: [NET_1], next_cursor: '' } })
      }

      // Ports
      if (url.includes('/ports')) {
        return route.fulfill({ status: 200, json: [] })
      }

      // Volume types
      if (url.includes('/volume-types')) {
        return route.fulfill({ status: 200, json: [] })
      }

      // Availability zones
      if (url.includes('/availability-zones')) {
        return route.fulfill({ status: 200, json: [AZ_1] })
      }

      // Default
      return route.fulfill({ status: 200, json: { items: [], next_cursor: '' } })
    })

    // --- STEP 1: Navigate to VM list ---
    await page.goto('/vms')
    await expect(page.getByText('VM 管理')).toBeVisible()
    await expect(page.getByText('VM がありません')).toBeVisible()

    // --- STEP 2: Open create dialog ---
    await page.locator('button', { hasText: 'VM を作成' }).click()
    await expect(page.locator('h3', { hasText: 'VM を作成' })).toBeVisible()

    // Fill in the name
    await page.getByTestId('vm-create-name').fill('lifecycle-vm')

    // Flavor should be pre-selected, verify it exists
    await expect(page.getByTestId('vm-create-flavor')).toBeVisible()

    // Network should be pre-selected
    await expect(page.getByTestId('vm-create-network')).toBeVisible()

    // AZ selector should be visible
    await expect(page.getByTestId('vm-create-az')).toBeVisible()
    // Select the AZ
    await page.getByTestId('vm-create-az').selectOption(AZ_1.id)

    // --- STEP 3: Submit create form ---
    await page.getByTestId('vm-create-submit').click()

    // Dialog should close and VM should appear in list
    await expect(page.locator('input[placeholder="my-vm"]')).not.toBeVisible()
    await expect(page.getByText('lifecycle-vm')).toBeVisible()

    // --- STEP 4: Start the VM ---
    // Find the start button for this VM
    const startBtn = page.getByTestId(`vm-start-${VM_ID}`)
    await expect(startBtn).toBeEnabled()
    await startBtn.click()

    // After starting, verify stop is enabled and start is disabled
    await expect(page.getByTestId(`vm-stop-${VM_ID}`)).toBeEnabled()
    await expect(page.getByTestId(`vm-start-${VM_ID}`)).toBeDisabled()

    // --- STEP 5: Stop the VM ---
    const stopBtn = page.getByTestId(`vm-stop-${VM_ID}`)
    await stopBtn.click()

    // After stopping, start should be enabled again
    await expect(page.getByTestId(`vm-start-${VM_ID}`)).toBeEnabled()
    await expect(page.getByTestId(`vm-stop-${VM_ID}`)).toBeDisabled()

    // --- STEP 6: Delete the VM ---
    const deleteBtn = page.getByTestId(`vm-delete-${VM_ID}`)
    await deleteBtn.click()

    // Confirm deletion in the dialog
    await expect(page.locator('h3', { hasText: 'VM を削除' })).toBeVisible()
    await page.locator('button', { hasText: '削除する' }).click()

    // VM should be gone from the list
    await expect(page.getByText('VM がありません')).toBeVisible()
  })

  test('VM 作成ダイアログに AZ セレクターが表示される', async ({ page }) => {
    await page.route('**/api/v1/**', async (route) => {
      const url = route.request().url()
      if (url.includes('/vms') && route.request().method() === 'GET') {
        return route.fulfill({ status: 200, json: { items: [], next_cursor: '' } })
      }
      if (url.includes('/flavors')) {
        return route.fulfill({ status: 200, json: { items: [FLAVOR_1], next_cursor: '' } })
      }
      if (url.match(/\/networks$/) && route.request().method() === 'GET') {
        return route.fulfill({ status: 200, json: { items: [NET_1], next_cursor: '' } })
      }
      if (url.includes('/volume-types')) {
        return route.fulfill({ status: 200, json: [] })
      }
      if (url.includes('/availability-zones')) {
        return route.fulfill({ status: 200, json: [AZ_1] })
      }
      return route.fulfill({ status: 200, json: { items: [], next_cursor: '' } })
    })

    await page.goto('/vms')
    await page.locator('button', { hasText: 'VM を作成' }).click()
    await expect(page.locator('h3', { hasText: 'VM を作成' })).toBeVisible()

    // AZ selector must be visible
    const azSelect = page.getByTestId('vm-create-az')
    await expect(azSelect).toBeVisible()

    // The AZ option should appear
    await expect(azSelect.locator('option', { hasText: 'zone-a' })).toHaveCount(1)
  })

  test('VM 一覧にフレーバー列と AZ 列が表示される', async ({ page }) => {
    await page.route('**/api/v1/**', async (route) => {
      const url = route.request().url()
      const method = route.request().method()
      if (url.match(/\/api\/v1\/vms$/) && method === 'GET') {
        return route.fulfill({
          status: 200,
          json: { items: [makeVm('stopped')], next_cursor: '' },
        })
      }
      if (url.includes('/flavors')) {
        return route.fulfill({ status: 200, json: { items: [FLAVOR_1], next_cursor: '' } })
      }
      if (url.includes('/availability-zones')) {
        return route.fulfill({ status: 200, json: [AZ_1] })
      }
      return route.fulfill({ status: 200, json: { items: [], next_cursor: '' } })
    })

    await page.goto('/vms')
    await expect(page.getByText('VM 管理')).toBeVisible()

    // Flavor column header
    await expect(page.getByText('フレーバー')).toBeVisible()
    // AZ column header
    await expect(page.getByText('AZ')).toBeVisible()

    // Flavor name in the row
    await expect(page.getByText('m1.small')).toBeVisible()
    // AZ name in the row
    await expect(page.getByText('zone-a')).toBeVisible()
  })
})
