import { useState, useEffect, useCallback } from 'react'
import { organizationsApi, type Organization, type Tenant } from '@/api/organizations'
import { quotasApi, type Quota, type UpdateQuotaRequest } from '@/api/quotas'
import { Button } from '@/components/Button'
import { Input } from '@/components/Input'
import { ErrorMessage } from '@/components/admin/ErrorMessage'

function QuotaEditor({ tenant }: { tenant: Tenant }) {
  const [quota, setQuota] = useState<Quota | null>(null)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState(false)
  const [form, setForm] = useState<UpdateQuotaRequest>({
    vcpus: 0,
    memory_mb: 0,
    vm_count: 0,
    volume_gb: 0,
  })

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    quotasApi
      .get(tenant.id)
      .then((q) => {
        setQuota(q)
        setForm({
          vcpus: q.vcpus,
          memory_mb: q.memory_mb,
          vm_count: q.vm_count,
          volume_gb: q.volume_gb,
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
    quotasApi
      .update(tenant.id, form)
      .then((q) => {
        setQuota(q)
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
      {error && <div className="mb-3"><ErrorMessage message={error} /></div>}
      {quota !== null && (
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label htmlFor={`quota-vcpus-${tenant.id}`} className="block text-xs font-medium mb-1 text-[var(--color-text-secondary)]">
              vCPU 上限
            </label>
            <Input
              id={`quota-vcpus-${tenant.id}`}
              type="number"
              min={0}
              value={form.vcpus}
              onChange={(e) =>
                setForm((f) => ({ ...f, vcpus: parseInt(e.target.value, 10) || 0 }))
              }
            />
          </div>
          <div>
            <label htmlFor={`quota-memory-${tenant.id}`} className="block text-xs font-medium mb-1 text-[var(--color-text-secondary)]">
              メモリ上限 (MB)
            </label>
            <Input
              id={`quota-memory-${tenant.id}`}
              type="number"
              min={0}
              value={form.memory_mb}
              onChange={(e) =>
                setForm((f) => ({ ...f, memory_mb: parseInt(e.target.value, 10) || 0 }))
              }
            />
          </div>
          <div>
            <label htmlFor={`quota-vm-count-${tenant.id}`} className="block text-xs font-medium mb-1 text-[var(--color-text-secondary)]">
              VM 数上限
            </label>
            <Input
              id={`quota-vm-count-${tenant.id}`}
              type="number"
              min={0}
              value={form.vm_count}
              onChange={(e) =>
                setForm((f) => ({ ...f, vm_count: parseInt(e.target.value, 10) || 0 }))
              }
            />
          </div>
          <div>
            <label htmlFor={`quota-volume-gb-${tenant.id}`} className="block text-xs font-medium mb-1 text-[var(--color-text-secondary)]">
              ボリューム容量上限 (GB)
            </label>
            <Input
              id={`quota-volume-gb-${tenant.id}`}
              type="number"
              min={0}
              value={form.volume_gb}
              onChange={(e) =>
                setForm((f) => ({ ...f, volume_gb: parseInt(e.target.value, 10) || 0 }))
              }
            />
          </div>
        </div>
      )}
      <div className="flex items-center justify-end gap-3 mt-3">
        {success && (
          <span className="text-sm text-[var(--color-success)]">保存しました</span>
        )}
        <Button size="sm" onClick={handleSave} disabled={saving || quota === null}>
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
              <div
                key={t.id}
                className="rounded border border-[var(--color-border)] p-3"
              >
                <div className="flex items-center justify-between">
                  <div>
                    <p className="font-medium text-sm">{t.name}</p>
                    <p className="text-xs font-mono text-[var(--color-text-secondary)]">{t.id}</p>
                  </div>
                  <Button
                    variant="secondary"
                    size="sm"
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
        <div className="rounded-lg border border-[var(--color-border)] bg-white p-8 text-center">
          <p className="text-sm text-[var(--color-text-secondary)]">組織がありません</p>
        </div>
      ) : (
        orgs.map((org) => <OrgSection key={org.id} org={org} />)
      )}
    </div>
  )
}
