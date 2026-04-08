import { test, expect } from '@playwright/test';

// S022-3: ログイン・認証フロー

test('ログイン: トークン未入力時はログインボタンが無効', async ({ page }) => {
  await page.goto('/login');
  await expect(page.getByTestId('login-button')).toBeDisabled();
});

test('ログイン: トークン入力後はログインボタンが有効', async ({ page }) => {
  await page.goto('/login');
  await page.getByTestId('token-input').fill('my-secret-token');
  await expect(page.getByTestId('login-button')).toBeEnabled();
});

test('ログイン: 256文字を超えるトークンはログインボタンが無効', async ({ page }) => {
  await page.goto('/login');
  await page.getByTestId('token-input').fill('a'.repeat(257));
  await expect(page.getByTestId('login-button')).toBeDisabled();
});

test('ログイン: 送信中はボタンが無効化されスピナーが表示される', async ({ page }) => {
  await page.route('/api/v1/auth/verify', route =>
    new Promise(resolve => setTimeout(() => resolve(route.fulfill({ json: { ok: true } })), 500))
  );
  await page.goto('/login');
  await page.getByTestId('token-input').fill('valid-token');
  await page.getByTestId('login-button').click();
  await expect(page.getByTestId('login-button')).toBeDisabled();
  await expect(page.getByTestId('login-spinner')).toBeVisible();
});

test('ログイン: 成功後にデフォルトのテナント管理画面にリダイレクト', async ({ page }) => {
  await page.route('/api/v1/auth/verify', route =>
    route.fulfill({ json: { ok: true } })
  );
  await page.goto('/login');
  await page.getByTestId('token-input').fill('valid-token');
  await page.getByTestId('login-button').click();
  await expect(page).toHaveURL('/');
});

test('ログイン: redirect パラメータがある場合、ログイン後に元のURLへ遷移', async ({ page }) => {
  await page.route('/api/v1/auth/verify', route =>
    route.fulfill({ json: { ok: true } })
  );
  await page.goto('/login?redirect=%2Fadmin%2Fhosts');
  await page.getByTestId('token-input').fill('valid-token');
  await page.getByTestId('login-button').click();
  await expect(page).toHaveURL('/admin/hosts');
});

test('ログイン: 未認証状態でページにアクセスするとログインページにリダイレクト', async ({ page }) => {
  await page.goto('/vms');
  await expect(page).toHaveURL(/\/login\?redirect=/);
});

test('ログイン: 401エラー時にインラインエラーメッセージが表示される', async ({ page }) => {
  await page.route('/api/v1/auth/verify', route =>
    route.fulfill({ status: 401, json: { error: 'unauthorized' } })
  );
  await page.goto('/login');
  await page.getByTestId('token-input').fill('invalid-token');
  await page.getByTestId('login-button').click();
  await expect(page.getByTestId('login-error-message')).toBeVisible();
  await expect(page.getByTestId('login-error-message')).toHaveText('トークンが無効です');
  await expect(page.getByTestId('login-button')).toBeEnabled();
});

test('ログイン: サーバーエラー時にトースト通知が表示される', async ({ page }) => {
  await page.route('/api/v1/auth/verify', route =>
    route.fulfill({ status: 500, json: { error: 'internal server error' } })
  );
  await page.goto('/login');
  await page.getByTestId('token-input').fill('some-token');
  await page.getByTestId('login-button').click();
  await expect(page.getByTestId('toast-error')).toBeVisible();
  await expect(page.getByTestId('toast-error')).toContainText('サーバーエラーが発生しました');
});

test('ログイン: ネットワーク障害時にトースト通知が表示される', async ({ page }) => {
  await page.route('/api/v1/auth/verify', route => route.abort('failed'));
  await page.goto('/login');
  await page.getByTestId('token-input').fill('some-token');
  await page.getByTestId('login-button').click();
  await expect(page.getByTestId('toast-error')).toBeVisible();
  await expect(page.getByTestId('toast-error')).toContainText('サーバーエラーが発生しました');
});
