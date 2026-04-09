import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { Toaster } from 'sonner'
import './index.css'
import { LoginPage } from './pages/LoginPage'
import { ProtectedRoute } from './components/ProtectedRoute'
import { AdminLayout } from './components/admin/AdminLayout'
import { OrganizationsPage } from './pages/admin/OrganizationsPage'
import { HostsPage } from './pages/admin/HostsPage'
import { StoragePage } from './pages/admin/StoragePage'
import { QuotasPage } from './pages/admin/QuotasPage'
import { DriftEventsPage } from './pages/admin/DriftEventsPage'
import { NetworkInfraPage } from './pages/admin/NetworkInfraPage'
import { TenantLayout } from './components/tenant/TenantLayout'
import { DashboardPage } from './pages/tenant/DashboardPage'
import { VmsPage } from './pages/tenant/VmsPage'
import { VmDetailPage } from './pages/tenant/VmDetailPage'
import { NetworksPage } from './pages/tenant/NetworksPage'
import { VolumesPage } from './pages/tenant/VolumesPage'
import { EgressPage } from './pages/tenant/EgressPage'
import { IngressPage } from './pages/tenant/IngressPage'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <Toaster richColors position="top-right" />
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route element={<ProtectedRoute />}>
          {/* Tenant routes */}
          <Route element={<TenantLayout />}>
            <Route path="/" element={<DashboardPage />} />
            <Route path="/vms" element={<VmsPage />} />
            <Route path="/vms/:id" element={<VmDetailPage />} />
            <Route path="/networks" element={<NetworksPage />} />
            <Route path="/volumes" element={<VolumesPage />} />
            <Route path="/egress" element={<EgressPage />} />
            <Route path="/ingress" element={<IngressPage />} />
          </Route>
          {/* Admin routes */}
          <Route path="/admin" element={<AdminLayout />}>
            <Route index element={<Navigate to="/admin/organizations" replace />} />
            <Route path="organizations" element={<OrganizationsPage />} />
            <Route path="hosts" element={<HostsPage />} />
            <Route path="storage" element={<StoragePage />} />
            <Route path="network" element={<NetworkInfraPage />} />
            <Route path="quotas" element={<QuotasPage />} />
            <Route path="drift-events" element={<DriftEventsPage />} />
          </Route>
        </Route>
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </BrowserRouter>
  </StrictMode>,
)
