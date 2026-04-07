import { useState, useEffect, useCallback } from 'react'
import {
  organizationsApi,
  type Organization,
  type Tenant,
  type RoleAssignment,
} from '@/api/organizations'
import { Button } from '@/components/Button'
import { Input } from '@/components/Input'
import { Dialog, ConfirmDialog } from '@/components/admin/Dialog'
import { ErrorMessage } from '@/components/admin/ErrorMessage'

// ---- Role Assignments panel -------------------------------------------------

function RoleAssignmentsPanel({ tenant }: { tenant: Tenant }) {
  const [assignments, setAssignments] = useState<RoleAssignment[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showAdd, setShowAdd] = useState(false)
  const [userId, setUserId] = useState('')
  const [role, setRole] = useState('member')
  const [saving, setSaving] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<RoleAssignment | null>(null)
  const [deleting, setDeleting] = useState(false)

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    organizationsApi
      .listRoleAssignments(tenant.id)
      .then(setAssignments)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [tenant.id])

  useEffect(() => {
    load()
  }, [load])

  const handleAdd = () => {
    if (!userId.trim()) return
    setSaving(true)
    organizationsApi
      .createRoleAssignment(tenant.id, { user_id: userId.trim(), role })
      .then(() => {
        setUserId('')
        setRole('member')
        setShowAdd(false)
        load()
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setSaving(false))
  }

  const handleDelete = () => {
    if (!deleteTarget) return
    setDeleting(true)
    organizationsApi
      .deleteRoleAssignment(tenant.id, deleteTarget.user_id)
      .then(() => {
        setDeleteTarget(null)
        load()
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setDeleting(false))
  }

  return (
    <div className="mt-3">
      <div className="flex items-center justify-between mb-2">
        <span className="text-xs font-medium text-[var(--color-text-secondary)] uppercase tracking-wide">
          ロール割り当て
        </span>
        <Button variant="secondary" size="sm" onClick={() => setShowAdd(true)}>
          + 追加
        </Button>
      </div>

      {error && <ErrorMessage message={error} />}

      {loading ? (
        <p className="text-sm text-[var(--color-text-secondary)]">読み込み中...</p>
      ) : assignments.length === 0 ? (
        <p className="text-sm text-[var(--color-text-secondary)]">割り当てなし</p>
      ) : (
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-[var(--color-border)]">
              <th className="text-left py-1 font-medium text-[var(--color-text-secondary)]">ユーザー ID</th>
              <th className="text-left py-1 font-medium text-[var(--color-text-secondary)]">ロール</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {assignments.map((a) => (
              <tr key={a.user_id} className="border-b border-[var(--color-border)] last:border-0">
                <td className="py-1.5 font-mono text-xs">{a.user_id}</td>
                <td className="py-1.5">{a.role}</td>
                <td className="py-1.5 text-right">
                  <Button variant="danger" size="sm" onClick={() => setDeleteTarget(a)}>
                    削除
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {/* Add role assignment dialog */}
      <Dialog open={showAdd} onClose={() => setShowAdd(false)} title="ロール割り当て追加">
        <div className="flex flex-col gap-3">
          <div>
            <label htmlFor="role-assignment-user-id" className="block text-sm font-medium mb-1">ユーザー ID</label>
            <Input
              id="role-assignment-user-id"
              value={userId}
              onChange={(e) => setUserId(e.target.value)}
              placeholder="user-uuid"
            />
          </div>
          <div>
            <label htmlFor="role-assignment-role" className="block text-sm font-medium mb-1">ロール</label>
            <select
              id="role-assignment-role"
              value={role}
              onChange={(e) => setRole(e.target.value)}
              className="flex h-9 w-full rounded border border-[var(--color-border)] bg-white px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
            >
              <option value="owner">owner</option>
              <option value="admin">admin</option>
              <option value="member">member</option>
              <option value="viewer">viewer</option>
            </select>
          </div>
          {error && <ErrorMessage message={error} />}
          <div className="flex justify-end gap-2 mt-1">
            <Button variant="secondary" size="sm" onClick={() => setShowAdd(false)} disabled={saving}>
              キャンセル
            </Button>
            <Button size="sm" onClick={handleAdd} disabled={saving}>
              {saving ? '追加中...' : '追加'}
            </Button>
          </div>
        </div>
      </Dialog>

      <ConfirmDialog
        open={deleteTarget !== null}
        onClose={() => setDeleteTarget(null)}
        onConfirm={handleDelete}
        title="ロール割り当て削除"
        description={`ユーザー "${deleteTarget?.user_id}" のロール割り当てを削除しますか？`}
        loading={deleting}
      />
    </div>
  )
}

// ---- Tenant list panel ------------------------------------------------------

function TenantsPanel({ org }: { org: Organization }) {
  const [tenants, setTenants] = useState<Tenant[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showAdd, setShowAdd] = useState(false)
  const [newName, setNewName] = useState('')
  const [saving, setSaving] = useState(false)
  const [selectedTenant, setSelectedTenant] = useState<Tenant | null>(null)

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    organizationsApi
      .listTenants(org.id)
      .then(setTenants)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [org.id])

  useEffect(() => {
    load()
  }, [load])

  const handleCreate = () => {
    if (!newName.trim()) return
    setSaving(true)
    organizationsApi
      .createTenant(org.id, { name: newName.trim() })
      .then(() => {
        setNewName('')
        setShowAdd(false)
        load()
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setSaving(false))
  }

  return (
    <div className="mt-2 pl-4 border-l-2 border-[var(--color-border)]">
      <div className="flex items-center justify-between mb-2">
        <span className="text-xs font-medium text-[var(--color-text-secondary)] uppercase tracking-wide">
          テナント
        </span>
        <Button variant="secondary" size="sm" onClick={() => setShowAdd(true)}>
          + テナント追加
        </Button>
      </div>

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
              className="rounded-lg border border-[var(--color-border)] bg-white p-4"
            >
              <div className="flex items-center justify-between">
                <div>
                  <p className="font-medium text-sm">{t.name}</p>
                  <p className="text-xs font-mono text-[var(--color-text-secondary)]">{t.id}</p>
                </div>
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => setSelectedTenant(selectedTenant?.id === t.id ? null : t)}
                >
                  {selectedTenant?.id === t.id ? '閉じる' : 'ロール管理'}
                </Button>
              </div>
              {selectedTenant?.id === t.id && <RoleAssignmentsPanel tenant={t} />}
            </div>
          ))}
        </div>
      )}

      {/* Add tenant dialog */}
      <Dialog open={showAdd} onClose={() => setShowAdd(false)} title="テナント作成">
        <div className="flex flex-col gap-3">
          <div>
            <label htmlFor="tenant-name" className="block text-sm font-medium mb-1">テナント名</label>
            <Input
              id="tenant-name"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              placeholder="my-tenant"
              onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
            />
          </div>
          {error && <ErrorMessage message={error} />}
          <div className="flex justify-end gap-2 mt-1">
            <Button variant="secondary" size="sm" onClick={() => setShowAdd(false)} disabled={saving}>
              キャンセル
            </Button>
            <Button size="sm" onClick={handleCreate} disabled={saving}>
              {saving ? '作成中...' : '作成'}
            </Button>
          </div>
        </div>
      </Dialog>
    </div>
  )
}

// ---- Main page --------------------------------------------------------------

export function OrganizationsPage() {
  const [orgs, setOrgs] = useState<Organization[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [showCreate, setShowCreate] = useState(false)
  const [newOrgName, setNewOrgName] = useState('')
  const [creating, setCreating] = useState(false)
  const [expandedOrg, setExpandedOrg] = useState<string | null>(null)

  const load = useCallback(() => {
    setLoading(true)
    setError(null)
    organizationsApi
      .list()
      .then(setOrgs)
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    load()
  }, [load])

  const handleCreate = () => {
    if (!newOrgName.trim()) return
    setCreating(true)
    organizationsApi
      .create({ name: newOrgName.trim() })
      .then(() => {
        setNewOrgName('')
        setShowCreate(false)
        load()
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => setCreating(false))
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h1 className="text-xl font-semibold text-[var(--color-text)]">組織・テナント管理</h1>
        <Button size="sm" onClick={() => setShowCreate(true)}>
          + 組織を作成
        </Button>
      </div>

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
        <div className="flex flex-col gap-4">
          {orgs.map((org) => (
            <div key={org.id} className="rounded-lg border border-[var(--color-border)] bg-white p-5">
              <div className="flex items-center justify-between">
                <div>
                  <p className="font-semibold text-[var(--color-text)]">{org.name}</p>
                  <p className="text-xs font-mono text-[var(--color-text-secondary)] mt-0.5">{org.id}</p>
                </div>
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => setExpandedOrg(expandedOrg === org.id ? null : org.id)}
                >
                  {expandedOrg === org.id ? 'テナントを隠す' : 'テナントを表示'}
                </Button>
              </div>
              {expandedOrg === org.id && <TenantsPanel org={org} />}
            </div>
          ))}
        </div>
      )}

      {/* Create org dialog */}
      <Dialog open={showCreate} onClose={() => setShowCreate(false)} title="組織を作成">
        <div className="flex flex-col gap-3">
          <div>
            <label htmlFor="org-name" className="block text-sm font-medium mb-1">組織名</label>
            <Input
              id="org-name"
              value={newOrgName}
              onChange={(e) => setNewOrgName(e.target.value)}
              placeholder="my-organization"
              onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
            />
          </div>
          <div className="flex justify-end gap-2 mt-1">
            <Button variant="secondary" size="sm" onClick={() => setShowCreate(false)} disabled={creating}>
              キャンセル
            </Button>
            <Button size="sm" onClick={handleCreate} disabled={creating}>
              {creating ? '作成中...' : '作成'}
            </Button>
          </div>
        </div>
      </Dialog>
    </div>
  )
}
