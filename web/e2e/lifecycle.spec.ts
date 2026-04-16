/**
 * lifecycle.spec.ts
 *
 * End-to-end tenant workflow lifecycle tests.
 *
 * Requirements:
 * - Requires a running server: set BASE_URL=http://localhost:<port>
 * - Uses real API — no mocks
 * - Reads seed IDs from .test-state.json written by global-setup.ts
 *
 * Skipped automatically when BASE_URL is not set.
 */
import { test, expect } from '@playwright/test'
import * as fs from 'fs'
import * as path from 'path'
import { fileURLToPath } from 'url'
import type { TestState } from './global-setup'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)

test.skip(!process.env.BASE_URL, 'BASE_URL not set — integration test requires make serve')

// ── Helpers ────────────────────────────────────────────────────────────────

function readState(): TestState {
  const stateFile = path.join(__dirname, '.test-state.json')
  if (!fs.existsSync(stateFile)) {
    throw new Error('.test-state.json not found — did global-setup run?')
  }
  return JSON.parse(fs.readFileSync(stateFile, 'utf-8')) as TestState
}

// ── Shared test state (carried across serial tests) ───────────────────────
const ctx: {
  networkId: string
  volumeId: string
  vmId: string
  egressId: string
  ingressId: string
} = {
  networkId: '',
  volumeId: '',
  vmId: '',
  egressId: '',
  ingressId: '',
}

// ── Serial lifecycle suite ─────────────────────────────────────────────────

test.describe.serial('テナントワークフロー ライフサイクル', () => {
  let state: TestState

  test.beforeAll(() => {
    state = readState()
  })

  test.beforeEach(async ({ page }) => {
    // Inject tenant ID before navigation
    await page.addInitScript((tenantId: string) => {
      localStorage.setItem('cirrus_token', 'dev-token')
      localStorage.setItem('cirrus_tenant_id', tenantId)
    }, state.tenantId)
  })

  // ── 1. ネットワーク作成 ──────────────────────────────────────────────────
  test('ネットワーク作成: ネットワーク一覧に表示される', async ({ page }) => {
    await page.goto('/networks')

    // Open create dialog
    await page.getByTestId('create-network-button').click()
    await expect(page.getByTestId('network-create-dialog')).toBeVisible()

    // Fill name
    await page.getByTestId('network-create-name').fill('e2e-lifecycle-net')
    await page.getByTestId('network-create-cidr').fill('10.99.0.0/24')
    await page.getByTestId('network-create-submit').click()

    // Dialog closes
    await expect(page.getByTestId('network-create-dialog')).not.toBeVisible()

    // Network appears in list — find the row by name text
    const netRow = page.getByText('e2e-lifecycle-net')
    await expect(netRow).toBeVisible()

    // Capture the network ID from data-testid attribute of the matching row
    const row = page.locator('[data-testid^="network-row-"]').filter({ hasText: 'e2e-lifecycle-net' })
    await expect(row).toBeVisible()
    const rowId = await row.getAttribute('data-testid')
    ctx.networkId = rowId?.replace('network-row-', '') ?? ''
    expect(ctx.networkId).toBeTruthy()
  })

  // ── 2. ボリューム作成 ──────────────────────────────────────────────────
  test('ボリューム作成: ボリューム一覧に表示される', async ({ page }) => {
    await page.goto('/volumes')

    await page.getByTestId('create-volume-button').click()
    await expect(page.getByTestId('volume-create-dialog')).toBeVisible()

    await page.getByTestId('volume-create-name').fill('e2e-lifecycle-vol')
    await page.getByTestId('volume-create-size').fill('10')
    await page.getByTestId('volume-create-submit').click()

    // Dialog should close (202 accepted)
    await expect(page.getByTestId('volume-create-dialog')).not.toBeVisible({ timeout: 10_000 })

    // Volume row should appear
    const row = page.locator('[data-testid^="volume-row-"]').filter({ hasText: 'e2e-lifecycle-vol' })
    await expect(row).toBeVisible({ timeout: 15_000 })
    const rowId = await row.getAttribute('data-testid')
    ctx.volumeId = rowId?.replace('volume-row-', '') ?? ''
    expect(ctx.volumeId).toBeTruthy()
  })

  // ── 3. VM 作成 ──────────────────────────────────────────────────────────
  test('VM 作成: VM 一覧に表示される', async ({ page }) => {
    await page.goto('/vms')

    await expect(page.getByText('VM 管理')).toBeVisible()

    await page.locator('button', { hasText: 'VM を作成' }).click()
    await expect(page.locator('h3', { hasText: 'VM を作成' })).toBeVisible()

    await page.getByTestId('vm-create-name').fill('e2e-lifecycle-vm')

    // Flavor and network should be auto-selected; just submit
    await page.getByTestId('vm-create-submit').click()

    // Dialog closes
    await expect(page.locator('h3', { hasText: 'VM を作成' })).not.toBeVisible({ timeout: 10_000 })

    // VM appears in list
    await expect(page.getByText('e2e-lifecycle-vm')).toBeVisible({ timeout: 15_000 })

    // Capture VM ID — find the start button within the row matching the VM name
    const vmRow = page.locator('tr').filter({ hasText: 'e2e-lifecycle-vm' })
    await expect(vmRow).toBeVisible()
    const startBtn = vmRow.locator('[data-testid^="vm-start-"]')
    const startId = await startBtn.getAttribute('data-testid')
    ctx.vmId = startId?.replace('vm-start-', '') ?? ''
    expect(ctx.vmId).toBeTruthy()
  })

  // ── 4. VM 起動 ──────────────────────────────────────────────────────────
  test('VM 起動: ステータスが running になる', async ({ page }) => {
    expect(ctx.vmId).toBeTruthy()
    await page.goto('/vms')

    const startBtn = page.getByTestId(`vm-start-${ctx.vmId}`)
    await expect(startBtn).toBeEnabled({ timeout: 10_000 })
    await startBtn.click()

    // After starting: stop button enabled, start button disabled
    await expect(page.getByTestId(`vm-stop-${ctx.vmId}`)).toBeEnabled({ timeout: 15_000 })
    await expect(page.getByTestId(`vm-start-${ctx.vmId}`)).toBeDisabled()
  })

  // ── 5. VM 停止 ──────────────────────────────────────────────────────────
  test('VM 停止: ステータスが stopped になる', async ({ page }) => {
    expect(ctx.vmId).toBeTruthy()
    await page.goto('/vms')

    const stopBtn = page.getByTestId(`vm-stop-${ctx.vmId}`)
    await expect(stopBtn).toBeEnabled({ timeout: 10_000 })
    await stopBtn.click()

    // After stopping: start button enabled
    await expect(page.getByTestId(`vm-start-${ctx.vmId}`)).toBeEnabled({ timeout: 15_000 })
    await expect(page.getByTestId(`vm-stop-${ctx.vmId}`)).toBeDisabled()
  })

  // ── 6. VM 再起動 ─────────────────────────────────────────────────────────
  test('VM 再起動: ステータスが running になる（reboot 後）', async ({ page }) => {
    expect(ctx.vmId).toBeTruthy()
    await page.goto('/vms')

    // First start the VM
    const startBtn = page.getByTestId(`vm-start-${ctx.vmId}`)
    await expect(startBtn).toBeEnabled({ timeout: 10_000 })
    await startBtn.click()
    await expect(page.getByTestId(`vm-stop-${ctx.vmId}`)).toBeEnabled({ timeout: 15_000 })

    // Then reboot
    const rebootBtn = page.getByTestId(`vm-reboot-${ctx.vmId}`)
    await expect(rebootBtn).toBeEnabled()
    await rebootBtn.click()

    // After reboot: stop button should eventually be enabled (running)
    await expect(page.getByTestId(`vm-stop-${ctx.vmId}`)).toBeEnabled({ timeout: 15_000 })
  })

  // ── 7. Egress 作成 ───────────────────────────────────────────────────────
  test('Egress 作成: Egress 一覧に表示される', async ({ page }) => {
    expect(ctx.networkId).toBeTruthy()
    await page.goto('/egress')

    // Wait for network selector to load and select the e2e network
    const networkSelect = page.locator('select').first()
    await expect(networkSelect).toBeVisible({ timeout: 10_000 })

    // Select the lifecycle network
    await networkSelect.selectOption({ value: ctx.networkId })

    // Click create
    const createBtn = page.getByTestId('egress-create-button')
    await expect(createBtn).toBeEnabled()
    await createBtn.click()

    await expect(page.getByTestId('egress-create-dialog')).toBeVisible()

    // nat_gateway is default, fill public IP
    await page.getByTestId('egress-public-ip-input').fill('198.51.100.1')
    await page.getByTestId('egress-create-submit').click()

    // Dialog closes, egress appears
    await expect(page.getByTestId('egress-create-dialog')).not.toBeVisible({ timeout: 10_000 })

    const rows = page.locator('[data-testid^="egress-row-"]')
    await expect(rows.first()).toBeVisible({ timeout: 10_000 })
    const firstId = await rows.first().getAttribute('data-testid')
    ctx.egressId = firstId?.replace('egress-row-', '') ?? ''
    expect(ctx.egressId).toBeTruthy()
  })

  // ── 8. Ingress 作成 ──────────────────────────────────────────────────────
  test('Ingress 作成: Ingress 一覧に表示される', async ({ page }) => {
    expect(ctx.networkId).toBeTruthy()
    expect(state.ipPoolId).toBeTruthy()

    await page.goto('/ingress')

    // Wait for the network selector and select e2e network
    const networkSelect = page.locator('select').first()
    await expect(networkSelect).toBeVisible({ timeout: 10_000 })
    await networkSelect.selectOption({ value: ctx.networkId })

    const createBtn = page.getByTestId('ingress-create-button')
    await expect(createBtn).toBeEnabled()
    await createBtn.click()

    await expect(page.getByTestId('ingress-create-dialog')).toBeVisible()

    // Select IP pool
    await page.getByTestId('ingress-ip-pool-select').selectOption({ value: state.ipPoolId! })

    // Public IP
    await page.getByTestId('ingress-public-ip-input').fill('198.51.100.2')

    // Target VM (optional — may skip if no VM available)
    if (ctx.vmId) {
      const vmSelect = page.getByTestId('ingress-target-vm-select')
      const optionCount = await vmSelect.locator('option').count()
      if (optionCount > 1) {
        await vmSelect.selectOption({ value: ctx.vmId })
      }
    }

    await page.getByTestId('ingress-create-submit').click()

    // Dialog closes, ingress appears
    await expect(page.getByTestId('ingress-create-dialog')).not.toBeVisible({ timeout: 10_000 })

    const rows = page.locator('[data-testid^="ingress-row-"]')
    await expect(rows.first()).toBeVisible({ timeout: 10_000 })
    const firstId = await rows.first().getAttribute('data-testid')
    ctx.ingressId = firstId?.replace('ingress-row-', '') ?? ''
    expect(ctx.ingressId).toBeTruthy()
  })

  // ── 9. 全リソース削除 ─────────────────────────────────────────────────────
  test('全リソース削除: VM/Egress/Ingress/Volume/Network が削除される', async ({ page }) => {
    // --- Delete VM ---
    if (ctx.vmId) {
      await page.goto('/vms')

      // Stop if running first
      const stopBtn = page.getByTestId(`vm-stop-${ctx.vmId}`)
      if (await stopBtn.isEnabled()) {
        await stopBtn.click()
        await expect(page.getByTestId(`vm-start-${ctx.vmId}`)).toBeEnabled({ timeout: 15_000 })
      }

      const deleteBtn = page.getByTestId(`vm-delete-${ctx.vmId}`)
      await expect(deleteBtn).toBeVisible()
      await deleteBtn.click()

      await expect(page.locator('h3', { hasText: 'VM を削除' })).toBeVisible()
      await page.getByTestId('vm-list-delete-confirm-button').click()

      await expect(page.getByText('e2e-lifecycle-vm')).not.toBeVisible({ timeout: 10_000 })
    }

    // --- Delete Ingress ---
    if (ctx.ingressId) {
      await page.goto('/ingress')

      const networkSelect = page.locator('select').first()
      await expect(networkSelect).toBeVisible({ timeout: 10_000 })
      await networkSelect.selectOption({ value: ctx.networkId })

      await page.getByTestId(`ingress-delete-button-${ctx.ingressId}`).click()
      await expect(page.getByTestId(`ingress-delete-confirm-${ctx.ingressId}`)).toBeVisible()
      await page.getByTestId(`ingress-delete-confirm-${ctx.ingressId}`).click()

      await expect(page.getByTestId(`ingress-row-${ctx.ingressId}`)).not.toBeVisible({ timeout: 10_000 })
    }

    // --- Delete Egress ---
    if (ctx.egressId) {
      await page.goto('/egress')

      const networkSelect = page.locator('select').first()
      await expect(networkSelect).toBeVisible({ timeout: 10_000 })
      await networkSelect.selectOption({ value: ctx.networkId })

      await page.getByTestId(`egress-delete-button-${ctx.egressId}`).click()
      await expect(page.getByTestId(`egress-delete-confirm-${ctx.egressId}`)).toBeVisible()
      await page.getByTestId(`egress-delete-confirm-${ctx.egressId}`).click()

      await expect(page.getByTestId(`egress-row-${ctx.egressId}`)).not.toBeVisible({ timeout: 10_000 })
    }

    // --- Delete Volume ---
    if (ctx.volumeId) {
      await page.goto('/volumes')

      const deleteBtn = page.getByTestId(`volume-delete-${ctx.volumeId}`)
      await expect(deleteBtn).toBeEnabled({ timeout: 15_000 })
      await deleteBtn.click()

      await expect(page.getByTestId('volume-delete-dialog')).toBeVisible()
      await page.getByTestId('volume-delete-confirm').click()

      await expect(page.getByTestId(`volume-row-${ctx.volumeId}`)).not.toBeVisible({ timeout: 10_000 })
    }

    // --- Delete Network ---
    if (ctx.networkId) {
      await page.goto('/networks')

      await page.getByTestId(`network-delete-${ctx.networkId}`).click()
      await expect(page.getByTestId('network-delete-dialog')).toBeVisible()
      await page.getByTestId('network-delete-confirm').click()

      await expect(page.getByText('e2e-lifecycle-net')).not.toBeVisible({ timeout: 10_000 })
    }
  })
})
