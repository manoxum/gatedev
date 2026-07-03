import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import type { HotspotBlockedDevice } from "@/components/hotspot/HotspotBlocklistCard";
import type { HotspotClient } from "@/components/hotspot/HotspotClientsCard";
import type { HotspotLimits, HotspotTraffic } from "@/components/hotspot/hotspot-limits-types";
import type { HotspotCredit } from "@/components/hotspot/hotspot-credit-types";

export interface HotspotStatus {
  running: boolean;
  status: string;
  channel?: string;
  band?: string;
}
export interface NetworkInterface {
  name: string;
  type: "wifi" | "other";
  state: string;
  speedMbps?: number;
}

export function useHotspotQueries() {
  const status = useQuery<HotspotStatus>({
    queryKey: ["hotspot", "status"],
    queryFn: () => api.get<HotspotStatus>("/hotspot/status"),
    refetchInterval: 5000,
  });
  const config = useQuery<Record<string, string>>({
    queryKey: ["hotspot", "config"],
    queryFn: () => api.get<Record<string, string>>("/hotspot/config"),
  });
  const interfaces = useQuery<NetworkInterface[]>({
    queryKey: ["hotspot", "interfaces"],
    queryFn: () => api.get<NetworkInterface[]>("/hotspot/interfaces"),
  });
  const clients = useQuery<HotspotClient[]>({
    queryKey: ["hotspot", "clients"],
    queryFn: () => api.get<HotspotClient[]>("/hotspot/clients"),
    refetchInterval: 5000,
    enabled: !!status.data?.running,
  });
  const blocklist = useQuery<HotspotBlockedDevice[]>({
    queryKey: ["hotspot", "blocklist"],
    queryFn: () => api.get<HotspotBlockedDevice[]>("/hotspot/blocklist"),
  });

  return { status, config, interfaces, clients, blocklist };
}

export function useGlobalLimits() {
  return useQuery<HotspotLimits>({
    queryKey: ["hotspot", "limits", "global"],
    queryFn: () => api.get<HotspotLimits>("/hotspot/limits/global"),
  });
}

export function useGlobalTraffic() {
  return useQuery<HotspotTraffic>({
    queryKey: ["hotspot", "limits", "global", "traffic"],
    queryFn: () => api.get<HotspotTraffic>("/hotspot/limits/global/traffic"),
    refetchInterval: 15000,
  });
}

export function useDeviceLimits(mac: string) {
  return useQuery<HotspotLimits>({
    queryKey: ["hotspot", "devices", mac, "limits"],
    queryFn: () => api.get<HotspotLimits>(`/hotspot/devices/${encodeURIComponent(mac)}/limits`),
    enabled: !!mac,
  });
}

export function useDeviceTraffic(mac: string) {
  return useQuery<HotspotTraffic>({
    queryKey: ["hotspot", "devices", mac, "traffic"],
    queryFn: () => api.get<HotspotTraffic>(`/hotspot/devices/${encodeURIComponent(mac)}/traffic`),
    enabled: !!mac,
    refetchInterval: 15000,
  });
}

export interface HotspotClientStats {
  mac: string;
  downloadBps: number;
  uploadBps: number;
}

export function useClientsStats(enabled: boolean) {
  return useQuery<HotspotClientStats[]>({
    queryKey: ["hotspot", "clients", "stats"],
    queryFn: () => api.get<HotspotClientStats[]>("/hotspot/clients/stats"),
    refetchInterval: 2500,
    enabled,
  });
}

export function useDeviceCredit(mac: string) {
  return useQuery<HotspotCredit>({
    queryKey: ["hotspot", "devices", mac, "credit"],
    queryFn: () => api.get<HotspotCredit>(`/hotspot/devices/${encodeURIComponent(mac)}/credit`),
    enabled: !!mac,
  });
}
