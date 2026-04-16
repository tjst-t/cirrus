/**
 * global-teardown.ts
 *
 * Reads .test-state.json and deletes resources created by global-setup.ts.
 * Idempotent — ignores 404 responses.
 *
 * No-op when BASE_URL is not set (ipPoolId / gatewayNodeId will be null).
 */
import * as fs from 'fs'
import * as path from 'path'
import { fileURLToPath } from 'url'
import type { TestState } from './global-setup'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)

const STATE_FILE = path.join(__dirname, '.test-state.json')

async function apiRequest(
  baseUrl: string,
  token: string,
  method: string,
  path: string,
): Promise<{ status: number }> {
  const res = await fetch(`${baseUrl}${path}`, {
    method,
    headers: {
      Authorization: `Bearer ${token}`,
    },
  })
  return { status: res.status }
}

export default async function globalTeardown() {
  if (!fs.existsSync(STATE_FILE)) {
    return
  }

  const state: TestState = JSON.parse(fs.readFileSync(STATE_FILE, 'utf-8'))

  const baseUrl = process.env.BASE_URL
  if (!baseUrl) {
    // Clean up state file even in no-op mode
    try { fs.unlinkSync(STATE_FILE) } catch { /* ignore */ }
    return
  }

  const token = process.env.E2E_TOKEN ?? 'dev-token'
  console.log('[global-teardown] Cleaning up e2e resources...')

  // Delete gateway node
  if (state.gatewayNodeId) {
    const res = await apiRequest(baseUrl, token, 'DELETE', `/api/v1/admin/gateway-nodes/${state.gatewayNodeId}`)
    if (res.status === 204 || res.status === 404) {
      console.log(`[global-teardown] Gateway node deleted: ${state.gatewayNodeId}`)
    } else {
      console.warn(`[global-teardown] Failed to delete gateway node ${state.gatewayNodeId}: ${res.status}`)
    }
  }

  // Delete IP pool
  if (state.ipPoolId) {
    const res = await apiRequest(baseUrl, token, 'DELETE', `/api/v1/admin/ip-pools/${state.ipPoolId}`)
    if (res.status === 204 || res.status === 404) {
      console.log(`[global-teardown] IP Pool deleted: ${state.ipPoolId}`)
    } else {
      console.warn(`[global-teardown] Failed to delete IP pool ${state.ipPoolId}: ${res.status}`)
    }
  }

  // Remove state file
  try { fs.unlinkSync(STATE_FILE) } catch { /* ignore */ }
  console.log('[global-teardown] Done')
}
