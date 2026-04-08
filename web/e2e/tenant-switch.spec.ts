import { test, expect } from '@playwright/test';

// S022-4: テナントコンテキスト切り替え

const TENANTS = [
  { id: 'tenant-1', name: 'テナントA' },
  { id: 'tenant-2', name: 'テナントB' },
];

// GET /api/v1/me/tenants returns paginated format: { items: [...], next_cursor: "" }
const MY_TENANTS_RESPONSE = { items: TENANTS, next_cursor: '' };

test.beforeEach(async ({ page }) => {
  // ログイン済み状態を模擬
  await page.addInitScript(() => {
    localStorage.setItem('auth_token', 'valid-token');
  });
});

test('テナント切り替え: ロード中はヘッダーにスピナーが表示される', async ({ page }) => {
  await page.route('/api/v1/me/tenants', route =>
    new Promise(resolve => setTimeout(() => resolve(route.fulfill({ json: MY_TENANTS_RESPONSE })), 500))
  );
  await page.goto('/');
  await expect(page.getByTestId('tenant-switcher-spinner')).toBeVisible();
});

test('テナント切り替え: テナント一覧取得成功後にドロップダウンが表示される', async ({ page }) => {
  await page.route('/api/v1/me/tenants', route =>
    route.fulfill({ json: MY_TENANTS_RESPONSE })
  );
  await page.goto('/');
  await expect(page.getByTestId('tenant-switcher')).toBeVisible();
  await expect(page.getByTestId('tenant-switcher')).toBeEnabled();
});

test('テナント切り替え: テナントが1件のみでもドロップダウンが表示される', async ({ page }) => {
  await page.route('/api/v1/me/tenants', route =>
    route.fulfill({ json: { items: [TENANTS[0]], next_cursor: '' } })
  );
  await page.goto('/');
  await expect(page.getByTestId('tenant-switcher')).toBeVisible();
});

test('テナント切り替え: テナント一覧取得失敗時にドロップダウンが無効化されトーストが表示される', async ({ page }) => {
  await page.route('/api/v1/me/tenants', route =>
    route.fulfill({ status: 500, json: { error: 'server error' } })
  );
  await page.goto('/');
  await expect(page.getByTestId('tenant-switcher')).toBeDisabled();
  await expect(page.getByTestId('toast-error')).toBeVisible();
});

test('テナント切り替え: 別テナントを選択するとページがリロードされる', async ({ page }) => {
  await page.route('/api/v1/me/tenants', route =>
    route.fulfill({ json: MY_TENANTS_RESPONSE })
  );
  await page.addInitScript(() => {
    localStorage.setItem('selected_tenant_id', 'tenant-1');
  });
  await page.goto('/');

  let reloaded = false;
  page.on('load', () => { reloaded = true; });

  await page.getByTestId('tenant-switcher').click();
  await page.getByTestId('tenant-option-tenant-2').click();

  await page.waitForLoadState('load');
  expect(reloaded).toBe(true);
  expect(await page.evaluate(() => localStorage.getItem('selected_tenant_id'))).toBe('tenant-2');
});

test('テナント切り替え: リロード後に「切り替えました」トーストが表示される', async ({ page }) => {
  await page.route('/api/v1/me/tenants', route =>
    route.fulfill({ json: MY_TENANTS_RESPONSE })
  );
  // テナント切り替え後のリロードを模擬（切り替えフラグを sessionStorage に保持）
  await page.addInitScript(() => {
    localStorage.setItem('selected_tenant_id', 'tenant-2');
    sessionStorage.setItem('tenant_just_switched', 'true');
  });
  await page.goto('/');
  await expect(page.getByTestId('toast-success')).toBeVisible();
  await expect(page.getByTestId('toast-success')).toContainText('テナントを切り替えました');
});

test('テナント切り替え: 現在選択中のテナント名がヘッダーに表示される', async ({ page }) => {
  await page.route('/api/v1/me/tenants', route =>
    route.fulfill({ json: MY_TENANTS_RESPONSE })
  );
  await page.addInitScript(() => {
    localStorage.setItem('selected_tenant_id', 'tenant-1');
  });
  await page.goto('/');
  await expect(page.getByTestId('tenant-switcher-label')).toHaveText('テナントA');
});

test('テナント切り替え: ドロップダウンは全ページのヘッダーに表示される', async ({ page }) => {
  await page.route('/api/v1/me/tenants', route =>
    route.fulfill({ json: MY_TENANTS_RESPONSE })
  );
  for (const path of ['/', '/vms', '/networks', '/volumes']) {
    await page.goto(path);
    await expect(page.getByTestId('tenant-switcher')).toBeVisible();
  }
});
