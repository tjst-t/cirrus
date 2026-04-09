import { useState, useEffect, useCallback } from 'react'
import { organizationsApi, type Organization, type Tenant } from '@/api/organizations'
import { quotaApi, type QuotaLimits } from '@/api/quota'
import { Button } from '@/components/Button'
import { Input } from '@/components/Input'
import { ErrorMessage } from '@/components/admin/ErrorMessage'

const QUOTA_FIELDS: Array<{ key: keyof QuotaLimits; label: string; slug: string }> = [
  { key: 'vcpus', label: 'vCPU 上限', slug: 'vcpus' },
  { key: 'memory_mb', label: 'メモリ上限 (MB)', slug: 'memory' },
  { key: 'vm_count', label: 'VM 数上限', slug: 'vm-count' },
  { key: 'volume_gb', label: 'ボリューム容量上限 (GB)', slug: 'volume-gb' },
  { key: 'networks', label: 'ネットワーク数上限', slug: 'networks' },
  { key: 'egresses', label: 'Egress 数上限', slug: 'egresses' },
  { key: 'ingresses', label: 'Ingress 数上限', slug: 'ingresses' },
]

function QuotaEditor({ tenant }: { tenant: Tenant }) {
  const [loaded, setLoaded] = useState(false)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState(false)
  const [form, setForm] = useState<QuotaLimits>({
    vcpus: 0,
    memory_mb: 0,
    vm_count: 0,
    volume_gb: 0,
    volumes: 0,
    snapshots: 0,
    networks: 0,
    egresses: 0,
    ingresses: 0,
  })

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    quotaApi
      .get(tenant.id)
      .then((q) => {
        setLoaded(true)
        setForm({
          vcpus: q.limits.vcpus,
          memory_mb: q.limits.memory_mb,
          vm_count: q.limits.vm_count,
          volume_gb: q.limits.volume_gb,
          volumes: q.limits.volumes,
          snapshots: q.limits.snapshots,
          networks: q.limits.networks,
          egresses: q.limits.egresses,
          ingresses: q.limits.ingresses,
        })
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [tenant.id])

  useEffect(() => {
    load()
  }, [load])

  useEffect(() => {
    if (!success) return
    const t = setTimeout(() => setSuccess(false), 3000)
    return () => clearTimeout(t)
  }, [success])

  const handleSave = () => {
    setSaving(true)
    setError(null)
    setSuccess(false)
    quotaApi
      .update(tenant.id, form)
      .then(() => {
        setSuccess(true)
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setSaving(false))
  }

  if (loading) {
    return <p className="text-sm text-[var(--color-text-secondary)]">読み込み中...</p>
  }

  return (
    <div className="mt-3">
      {error && (
        <div className="mb-3">
          <ErrorMessage data-testid={`quota-save-error-${tenant.id}`} message={error} />
        </div>
      )}
      {loaded && (
        <div className="grid grid-cols-2 gap-3">
          {QUOTA_FIELDS.map(({ key, label, slug }) => (
            <div key={key}>
              <label
                htmlFor={`quota-${slug}-${tenant.id}`}
                className="block text-xs font-medium mb-1 text-[var(--color-text-secondary)]"
              >
                {label}
              </label>
              <Input
                id={`quota-${slug}-${tenant.id}`}
                data-testid={`quota-${slug}-${tenant.id}`}
                type="number"
                min={0}
                value={form[key]}
                onChange={(e) =>
                  setForm((f) => ({ ...f, [key]: parseInt(e.target.value, 10) || 0 }))
                }
              />
            </div>
          ))}
        </div>
      )}
      <div className="flex items-center justify-end gap-3 mt-3">
        {success && (
          <span
            data-testid={`quota-save-success-${tenant.id}`}
            className="text-sm text-[var(--color-success)]"
          >
            保存しました
          </span>
        )}
        <Button
          data-testid={`quota-save-button-${tenant.id}`}
          size="sm"
          onClick={handleSave}
          disabled={saving || !loaded}
        >
          {saving ? '保存中...' : '保存'}
        </Button>
      </div>
    </div>
  )
}

function OrgSection({ org }: { org: Organization }) {
  const [tenants, setTenants] = useState<Tenant[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [expanded, setExpanded] = useState<string | null>(null)

  useEffect(() => {
    setLoading(true)
    organizationsApi
      .listTenants(org.id)
      .then(setTenants)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [org.id])

  return (
    <div className="rounded-lg border border-[var(--color-border)] bg-white overflow-hidden mb-4">
      <div className="px-5 py-3 border-b border-[var(--color-border)] bg-[var(--color-bg-secondary)]">
        <p className="font-semibold text-[var(--color-text)]">{org.name}</p>
        <p className="text-xs font-mono text-[var(--color-text-secondary)]">{org.id}</p>
      </div>
      <div className="p-4">
        {error && <ErrorMessage message={error} />}
        {loading ? (
          <p className="text-sm text-[var(--color-text-secondary)]">読み込み中...</p>
        ) : tenants.length === 0 ? (
          <p className="text-sm text-[var(--color-text-secondary)]">テナントなし</p>
        ) : (
          <div className="flex flex-col gap-3">
            {tenants.map((t) => (
              <div key={t.id} className="rounded border border-[var(--color-border)] p-3">
                <div className="flex items-center justify-between">
                  <div>
                    <p className="font-medium text-sm">{t.name}</p>
                    <p className="text-xs font-mono text-[var(--color-text-secondary)]">{t.id}</p>
                  </div>
                  <Button
                    variant="secondary"
                    size="sm"
                    data-testid={`quota-edit-button-${t.id}`}
                    onClick={() => setExpanded(expanded === t.id ? null : t.id)}
                  >
                    {expanded === t.id ? '閉じる' : 'Quota 編集'}
                  </Button>
                </div>
                {expanded === t.id && <QuotaEditor tenant={t} />}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

export function QuotasPage() {
  const [orgs, setOrgs] = useState<Organization[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setLoading(true)
    organizationsApi
      .list()
      .then(setOrgs)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  return (
    <div>
      <h1 className="text-xl font-semibold text-[var(--color-text)] mb-6">Quota 設定</h1>

      {error && (
        <div className="mb-4">
          <ErrorMessage message={error} />
        </div>
      )}

      {loading ? (
        <p className="text-sm text-[var(--color-text-secondary)]">読み込み中...</p>
      ) : orgs.length === 0 ? (
        <div
          data-testid="empty-orgs-quota"
          className="rounded-lg border border-[var(--color-border)] bg-white p-8 text-center"
        >
          <p className="text-sm text-[var(--color-text-secondary)]">組織がありません</p>
        </div>
      ) : (
        orgs.map((org) => <OrgSection key={org.id} org={org} />)
      )}
    </div>
  )
}
