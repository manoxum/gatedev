import { Routes, Route } from "react-router-dom";
import { RequireAuth } from "@/components/RequireAuth";
import { AppLayout } from "@/components/layout/AppLayout";
import { LoginPage } from "@/pages/Login";
import { PortalPage } from "@/pages/Portal";
import { SetupPage } from "@/pages/Setup";
import { DashboardPage } from "@/pages/Dashboard";
import { HotspotPage } from "@/pages/Hotspot";
import { HotspotDeviceDetailPage } from "@/pages/HotspotDeviceDetail";
import { HotspotVoucherBatchDetailPage } from "@/pages/HotspotVoucherBatchDetail";
import { DnsPage } from "@/pages/Dns";
import { CertificatesPage } from "@/pages/Certificates";
import { BindnetsPage } from "@/pages/Bindnets";
import { BindnetDetailPage } from "@/pages/BindnetDetail";
import { SettingsPage } from "@/pages/Settings";

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route path="/portal" element={<PortalPage />} />
      <Route
        path="/setup"
        element={
          <RequireAuth>
            <SetupPage />
          </RequireAuth>
        }
      />
      <Route
        element={
          <RequireAuth>
            <AppLayout />
          </RequireAuth>
        }
      >
        <Route index element={<DashboardPage />} />
        <Route path="hotspot" element={<HotspotPage />} />
        <Route path="hotspot/devices/:mac" element={<HotspotDeviceDetailPage />} />
        <Route path="hotspot/vouchers/batches/:id" element={<HotspotVoucherBatchDetailPage />} />
        <Route path="dns" element={<DnsPage />} />
        <Route path="bindnets" element={<BindnetsPage />} />
        <Route path="bindnets/:nodeId" element={<BindnetDetailPage />} />
        <Route path="certificates" element={<CertificatesPage />} />
        <Route path="settings" element={<SettingsPage />} />
      </Route>
    </Routes>
  );
}
