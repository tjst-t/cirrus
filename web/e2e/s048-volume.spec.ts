import { test, expect } from '@playwright/test'

const VOLUME_ID = 'vol-0000-0000-0000-000000000001'
const JOB_ID = 'job-0000-0000-0000-000000000001'
const TENANT_1 = { id: 'test-tenant-id', name: 'Test Tenant', organization_id: 'org-1', created_at: '2024-01-01T00:00:00Z' }

const VOL_AVAILABLE = {
  id: VOLUME_ID,
  tenant_id: TENANT_1.id,
  name: 'my-volume',
  size_gb: 20,
  state: 'available',
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
}

const VOL_IN_USE = { ...VOL_AVAILABLE, state: 'in_use' }

const VT_1 = {
  id: 'vt-0000-0000-0000-000000000001',
  name: 'ssd',
  description: 'SSD Volume',
  required_capabilities: [],
  is_public: true,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
}

test.describe('S048-2: ボリューム管理', () => {
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
    await page.route('**/api/v1/volume-types', (route) =>
      route.fulfill({ status: 200, json: [VT_1] })
    )
  })

  test('ボリューム一覧: ボリュームが存在しない場合に空状態を表示する', async ({ page }) => {
    await page.route('**/api/v1/volumes', (route) =>
      route.fulfill({ status: 200, json: { items: [], next_cursor: '' } })
    )
    await page.goto('/volumes')
    await expect(page.getByTestId('volume-empty-state')).toBeVisible()
    await expect(page.getByTestId('create-volume-button')).toBeEnabled()
  })

  test('ボリューム一覧: stateバッジが正しく表示される', async ({ page }) => {
    const volumes = [
      { ...VOL_AVAILABLE, id: 'vol-1', name: 'vol-available', state: 'available' },
      { ...VOL_AVAILABLE, id: 'vol-2', name: 'vol-in-use', state: 'in_use' },
      { ...VOL_AVAILABLE, id: 'vol-3', name: 'vol-creating', state: 'creating' },
    ]
    await page.route('**/api/v1/volumes', (route) =>
      route.fulfill({ status: 200, json: { items: volumes, next_cursor: '' } })
    )
    await page.goto('/volumes')
    await expect(page.getByTestId('volume-state-vol-1')).toContainText('利用可能')
    await expect(page.getByTestId('volume-state-vol-2')).toContainText('使用中')
    await expect(page.getByTestId('volume-state-vol-3')).toContainText('作成中')
  })

  test('ボリューム作成: 名前とサイズを入力して作成できる（202 job_id）', async ({ page }) => {
    let volumeCreated = false
    await page.route('**/api/v1/volumes', async (route) => {
      if (route.request().method() === 'GET') {
        return route.fulfill({
          status: 200,
          json: { items: volumeCreated ? [VOL_AVAILABLE] : [], next_cursor: '' },
        })
      }
      if (route.request().method() === 'POST') {
        const body = JSON.parse(route.request().postData() ?? '{}')
        expect(body.name).toBe('my-volume')
        expect(body.size_gb).toBe(20)
        volumeCreated = true
        return route.fulfill({ status: 202, json: { job_id: JOB_ID } })
      }
    })

    await page.goto('/volumes')
    await expect(page.getByTestId('volume-empty-state')).toBeVisible()

    await page.getByTestId('create-volume-button').click()
    await expect(page.getByTestId('volume-create-dialog')).toBeVisible()

    await page.getByTestId('volume-create-name').fill('my-volume')
    await page.getByTestId('volume-create-size').fill('20')
    await page.getByTestId('volume-create-submit').click()

    await expect(page.getByTestId('volume-create-dialog')).not.toBeVisible()
    await expect(page.getByTestId(`volume-row-${VOLUME_ID}`)).toBeVisible()
  })

  test('ボリューム作成: ボリュームタイプを選択して作成できる', async ({ page }) => {
    await page.route('**/api/v1/volumes', async (route) => {
      if (route.request().method() === 'GET') {
        return route.fulfill({ status: 200, json: { items: [VOL_AVAILABLE], next_cursor: '' } })
      }
      if (route.request().method() === 'POST') {
        const body = JSON.parse(route.request().postData() ?? '{}')
        expect(body.volume_type_id).toBe(VT_1.id)
        return route.fulfill({ status: 202, json: { job_id: JOB_ID } })
      }
    })

    await page.goto('/volumes')
    await page.getByTestId('create-volume-button').click()
    await page.getByTestId('volume-create-name').fill('ssd-volume')
    await page.getByTestId('volume-create-type').selectOption(VT_1.id)
    await page.getByTestId('volume-create-submit').click()
    await expect(page.getByTestId('volume-create-dialog')).not.toBeVisible()
  })

  test('ボリュームリサイズ: new_size_gbを指定してリサイズできる', async ({ page }) => {
    await page.route('**/api/v1/volumes', (route) =>
      route.fulfill({ status: 200, json: { items: [VOL_AVAILABLE], next_cursor: '' } })
    )
    await page.route(`**/api/v1/volumes/${VOLUME_ID}/resize`, async (route) => {
      const body = JSON.parse(route.request().postData() ?? '{}')
      expect(body.new_size_gb).toBe(40)
      return route.fulfill({ status: 202, json: { job_id: JOB_ID } })
    })

    await page.goto('/volumes')
    await expect(page.getByTestId(`volume-row-${VOLUME_ID}`)).toBeVisible()

    await page.getByTestId(`volume-resize-${VOLUME_ID}`).click()
    await expect(page.getByTestId('volume-resize-dialog')).toBeVisible()

    await page.getByTestId('volume-resize-size').fill('40')
    await page.getByTestId('volume-resize-submit').click()
    await expect(page.getByTestId('volume-resize-dialog')).not.toBeVisible()
  })

  test('ボリュームリサイズ: 現在のサイズ以下を指定するとエラーになる', async ({ page }) => {
    await page.route('**/api/v1/volumes', (route) =>
      route.fulfill({ status: 200, json: { items: [VOL_AVAILABLE], next_cursor: '' } })
    )

    await page.goto('/volumes')
    await page.getByTestId(`volume-resize-${VOLUME_ID}`).click()
    await page.getByTestId('volume-resize-size').fill('10')
    await page.getByTestId('volume-resize-submit').click()
    // エラーが表示されダイアログは閉じない
    await expect(page.getByTestId('volume-resize-dialog')).toBeVisible()
    await expect(page.getByTestId('volume-resize-error')).toBeVisible()
  })

  test('ボリューム削除: available状態のボリュームを確認後に削除できる', async ({ page }) => {
    let deleted = false
    await page.route('**/api/v1/volumes', (route) =>
      route.fulfill({
        status: 200,
        json: { items: deleted ? [] : [VOL_AVAILABLE], next_cursor: '' },
      })
    )
    await page.route(`**/api/v1/volumes/${VOLUME_ID}`, async (route) => {
      if (route.request().method() === 'DELETE') {
        deleted = true
        return route.fulfill({ status: 202, json: { job_id: JOB_ID } })
      }
    })

    await page.goto('/volumes')
    await expect(page.getByTestId(`volume-row-${VOLUME_ID}`)).toBeVisible()

    await page.getByTestId(`volume-delete-${VOLUME_ID}`).click()
    await expect(page.getByTestId('volume-delete-dialog')).toBeVisible()
    await page.getByTestId('volume-delete-confirm').click()

    await expect(page.getByTestId('volume-empty-state')).toBeVisible()
  })

  test('ボリューム削除: in_use状態のボリュームは削除ボタンが無効化される', async ({ page }) => {
    await page.route('**/api/v1/volumes', (route) =>
      route.fulfill({ status: 200, json: { items: [VOL_IN_USE], next_cursor: '' } })
    )
    await page.goto('/volumes')
    await expect(page.getByTestId(`volume-delete-${VOLUME_ID}`)).toBeDisabled()
  })

  test('ボリューム一覧: API失敗時にエラーメッセージを表示する', async ({ page }) => {
    await page.route('**/api/v1/volumes', (route) =>
      route.fulfill({ status: 500, json: { error: 'internal server error' } })
    )
    await page.goto('/volumes')
    await expect(page.getByTestId('error-message')).toBeVisible()
  })
})
