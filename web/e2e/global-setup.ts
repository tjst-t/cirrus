/**
 * global-setup.ts
 *
 * Seeds test-specific resources via the API when BASE_URL is set.
 * Writes created resource IDs to web/e2e/.test-state.json so that
 * lifecycle.spec.ts (and teardown) can reference them.
 *
 * No-op when BASE_URL is not set (mock-only runs).
 */
import * as fs from 'fs'
import * as path from 'path'
import { fileURLToPath } from 'url'

const __filename = fileURLToPath(import.meta.url)
const __dirname = path.dirname(__filename)

export interface TestState {
  tenantId: string
  ipPoolId: string | null
  gatewayNodeId: string | null
}

const STATE_FILE = path.join(__dirname, '.test-state.json')

async function apiRequest(
  baseUrl: string,
  token: string,
  method: string,
  path: string,
  body?: unknown,
): Promise<{ status: number; json: unknown }> {
  const res = await fetch(`${baseUrl}${path}`, {
    method,
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
    body: body !== undefined ? JSON.stringify(body) : undefined,
  })
  let json: unknown = null
  try {
    json = await res.json()
  } catch {
    // no-op: some responses (204) have no body
  }
  return { status: res.status, json }
}

export default async function globalSetup() {
  const baseUrl = process.env.BASE_URL
  if (!baseUrl) {
    // No server available — write an empty state so teardown can be a no-op too
    const state: TestState = {
      tenantId: process.env.E2E_TENANT_ID ?? '4af01cf9-7325-4742-bf30-f1852368c1e8',
      ipPoolId: null,
      gatewayNodeId: null,
    }
    fs.writeFileSync(STATE_FILE, JSON.stringify(state, null, 2))
    return
  }

  const token = process.env.E2E_TOKEN ?? 'dev-token'
  const tenantId = process.env.E2E_TENANT_ID ?? '4af01cf9-7325-4742-bf30-f1852368c1e8'

  console.log(`[global-setup] BASE_URL=${baseUrl}  tenant=${tenantId}`)

  // ── 1. AZ: ensure default-az exists (read-only check) ─────────────────────
  const azRes = await apiRequest(baseUrl, token, 'GET', '/api/v1/admin/availability-zones')
  const azList = (azRes.json as { items?: Array<{ id: string; name: string }> })?.items ?? []
  const defaultAz = azList.find((a) => a.name === 'default-az') ?? azList[0]
  if (defaultAz) {
    console.log(`[global-setup] AZ: ${defaultAz.name} (${defaultAz.id})`)
  } else {
    console.warn('[global-setup] No AZ found — some lifecycle tests may fail')
  }

  // ── 2. Flavor: ensure m1-small exists (read-only check) ───────────────────
  const flavorRes = await apiRequest(baseUrl, token, 'GET', '/api/v1/flavors')
  const flavorList = (flavorRes.json as { items?: Array<{ id: string; name: string }> })?.items ?? []
  const defaultFlavor = flavorList.find((f) => f.name === 'm1-small') ?? flavorList[0]
  if (defaultFlavor) {
    console.log(`[global-setup] Flavor: ${defaultFlavor.name} (${defaultFlavor.id})`)
  } else {
    console.warn('[global-setup] No flavor found — lifecycle tests may fail')
  }

  // ── 3. Quota: set test-tenant quota ───────────────────────────────────────
  await apiRequest(baseUrl, token, 'PUT', `/api/v1/tenants/${tenantId}/quota`, {
    vcpus: 32,
    memory_mb: 65536,
    vm_count: 20,
    volume_gb: 2000,
    volumes: 40,
    snapshots: 80,
    networks: 10,
    egresses: 10,
    ingresses: 20,
  })
  console.log(`[global-setup] Quota set for tenant ${tenantId}`)

  // ── 4. IP Pool: create e2e-pool (idempotent) ──────────────────────────────
  const poolListRes = await apiRequest(baseUrl, token, 'GET', '/api/v1/admin/ip-pools')
  const poolList = (poolListRes.json as Array<{ id: string; name: string }>) ?? []
  let ipPoolId: string | null = null

  const existingPool = poolList.find((p) => p.name === 'e2e-pool')
  if (existingPool) {
    ipPoolId = existingPool.id
    console.log(`[global-setup] IP Pool already exists: ${ipPoolId}`)
  } else {
    const createPoolRes = await apiRequest(baseUrl, token, 'POST', '/api/v1/admin/ip-pools', {
      name: 'e2e-pool',
      cidr: '198.51.100.0/24',
      description: 'E2E test IP pool',
    })
    if (createPoolRes.status === 201) {
      ipPoolId = (createPoolRes.json as { id: string }).id
      console.log(`[global-setup] IP Pool created: ${ipPoolId}`)
    } else {
      console.warn(`[global-setup] Failed to create IP pool: ${createPoolRes.status}`)
    }
  }

  // ── 5. Gateway Node: create for first active host (idempotent) ────────────
  // GatewayNode has no name field; idempotency is by host_id.
  let gatewayNodeId: string | null = null

  const hostRes = await apiRequest(baseUrl, token, 'GET', '/api/v1/hosts?state=active')
  const hostList = (hostRes.json as Array<{ id: string }>) ?? []
  const firstHost = hostList[0]

  if (firstHost) {
    const gwListRes = await apiRequest(baseUrl, token, 'GET', '/api/v1/admin/gateway-nodes')
    const gwList = (gwListRes.json as Array<{ id: string; host_id: string }>) ?? []
    const existingGw = gwList.find((g) => g.host_id === firstHost.id)

    if (existingGw) {
      gatewayNodeId = existingGw.id
      console.log(`[global-setup] Gateway node already exists: ${gatewayNodeId}`)
    } else {
      const createGwRes = await apiRequest(baseUrl, token, 'POST', '/api/v1/admin/gateway-nodes', {
        host_id: firstHost.id,
        external_ip: '203.0.113.1',
        internal_ip: '10.100.0.200',
      })
      if (createGwRes.status === 201) {
        gatewayNodeId = (createGwRes.json as { id: string }).id
        console.log(`[global-setup] Gateway node created: ${gatewayNodeId}`)
      } else {
        console.warn(`[global-setup] Failed to create gateway node: ${createGwRes.status}`)
      }
    }
  } else {
    console.warn('[global-setup] No active hosts found — skipping GW node creation')
  }

  // ── Write state ────────────────────────────────────────────────────────────
  const state: TestState = { tenantId, ipPoolId, gatewayNodeId }
  fs.writeFileSync(STATE_FILE, JSON.stringify(state, null, 2))
  console.log(`[global-setup] State written to ${STATE_FILE}`)
}
