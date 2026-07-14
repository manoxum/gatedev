import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import type { HotspotBlockedDevice } from "@/components/hotspot/HotspotBlocklistCard";
import type { HotspotClient } from "@/components/hotspot/HotspotClientsCard";
import type {
  HotspotGlobalLimits,
  HotspotDeviceLimitsResponse,
  HotspotTraffic,
  HotspotDeviceQuotaPeriodUsage,
} from "@/components/hotspot/hotspot-limits-types";
import type { HotspotCredit, HotspotCreditHistoryEntry } from "@/components/hotspot/hotspot-credit-types";
import type { HotspotSessionConsumptionEntry } from "@/components/hotspot/hotspot-session-types";

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

export interface HotspotKnownDevice {
  mac: string;
  vendor?: string;
  deviceName?: string;
  osName?: string;
  alias?: string;
  firstSeenAt?: string;
  lastSeenAt?: string;
  connected: boolean;
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
  const knownDevices = useQuery<HotspotKnownDevice[]>({
    queryKey: ["hotspot", "devices", "known"],
    queryFn: () => api.get<HotspotKnownDevice[]>("/hotspot/devices/known"),
  });

  return { status, config, interfaces, clients, blocklist, knownDevices };
}

export function useGlobalLimits() {
  return useQuery<HotspotGlobalLimits>({
    queryKey: ["hotspot", "limits", "global"],
    queryFn: () => api.get<HotspotGlobalLimits>("/hotspot/limits/global"),
  });
}

export function useGlobalTraffic() {
  return useQuery<HotspotTraffic>({
    queryKey: ["hotspot", "limits", "global", "traffic"],
    queryFn: () => api.get<HotspotTraffic>("/hotspot/limits/global/traffic"),
    refetchInterval: 15000,
  });
}

// Sempre os limites EFETIVOS (herdados do perfil, ou o override do
// proprio dispositivo quando o perfil vinculado e "custom"), com
// profileName/profileLimitType pra UI decidir entre resumo herdado
// so-leitura ou formulario editavel - ver DeviceLimitsTab.tsx.
export function useDeviceLimits(mac: string) {
  return useQuery<HotspotDeviceLimitsResponse>({
    queryKey: ["hotspot", "devices", mac, "limits"],
    queryFn: () => api.get<HotspotDeviceLimitsResponse>(`/hotspot/devices/${encodeURIComponent(mac)}/limits`),
    enabled: !!mac,
  });
}

export interface HotspotDeviceStats {
  downloadBps: number;
  uploadBps: number;
}

// Velocidade ao vivo do dispositivo - poll curto (2.5s) em
// /api/hotspot/devices/{mac}/stats, que ja devolve bytes/segundo
// prontos (o backend calcula o delta comparando duas leituras
// sucessivas dos contadores absolutos do worker). Usada pelos
// velocimetros compactos embutidos no cabecalho de
// DeviceSpeedChart.tsx (o "agora" ao lado da tendencia no tempo).
export function useDeviceStats(mac: string) {
  return useQuery<HotspotDeviceStats>({
    queryKey: ["hotspot", "devices", mac, "stats"],
    queryFn: () => api.get<HotspotDeviceStats>(`/hotspot/devices/${encodeURIComponent(mac)}/stats`),
    enabled: !!mac,
    refetchInterval: 2500,
  });
}

// Ate 3 periodos de cota (diario/semanal/mensal) - so os efetivamente
// configurados (ver hotspot_device_quota.go). Substitui o antigo
// useDeviceTraffic (endpoint removido - so tinha bytes acumulados do
// periodo de cota corrente, nao uma taxa/velocidade).
export function useDeviceQuotaPeriods(mac: string) {
  return useQuery<HotspotDeviceQuotaPeriodUsage[]>({
    queryKey: ["hotspot", "devices", mac, "quota"],
    queryFn: () => api.get<HotspotDeviceQuotaPeriodUsage[]>(`/hotspot/devices/${encodeURIComponent(mac)}/quota`),
    enabled: !!mac,
    refetchInterval: 15000,
  });
}

export interface HotspotSpeedSample {
  at: string;
  downloadBps: number;
  uploadBps: number;
}

// refetchIntervalForWindow ajusta a cadencia de poll ao tamanho da
// janela escolhida - faz sentido redesenhar a cada 1s pra janelas
// curtas (1-15min), mas buscar/redesenhar milhares de pontos toda vez
// pra uma janela de 6h/12h/1 dia so desperdica rede/CPU sem diferenca
// visivel (a tendencia mal muda segundo a segundo nessa escala).
function refetchIntervalForWindow(minutes: number): number {
  if (minutes <= 15) return 1000;
  if (minutes <= 60) return 5000;
  return 30_000;
}

// Busca exatamente a janela selecionada (nao mais um maximo fixo
// filtrado no cliente) - o backend retem ate 1 dia por MAC (ver
// hotspot_device_speed_history.go), buscar so o que a UI mostra evita
// transferir/redesenhar pontos demais numa janela longa.
export function useDeviceSpeedHistory(mac: string, minutes: number) {
  return useQuery<HotspotSpeedSample[]>({
    queryKey: ["hotspot", "devices", mac, "speed-history", minutes],
    queryFn: () =>
      api.get<HotspotSpeedSample[]>(
        `/hotspot/devices/${encodeURIComponent(mac)}/speed-history?minutes=${minutes}`,
      ),
    enabled: !!mac,
    refetchInterval: refetchIntervalForWindow(minutes),
  });
}

// Velocidade ao vivo agregada de todo o hotspot (nao um dispositivo) -
// equivalente global de useDeviceStats, mesmo poll de 2.5s, backend em
// GET /api/hotspot/stats/global (globalLiveStats, hotspot_stats.go).
export function useGlobalStats() {
  return useQuery<HotspotDeviceStats>({
    queryKey: ["hotspot", "stats", "global"],
    queryFn: () => api.get<HotspotDeviceStats>("/hotspot/stats/global"),
    refetchInterval: 2500,
  });
}

// Historico de velocidade agregada de todo o hotspot - equivalente
// global de useDeviceSpeedHistory, mesma janela sob demanda e mesma
// cadencia adaptativa de poll.
export function useGlobalSpeedHistory(minutes: number) {
  return useQuery<HotspotSpeedSample[]>({
    queryKey: ["hotspot", "limits", "global", "speed-history", minutes],
    queryFn: () => api.get<HotspotSpeedSample[]>(`/hotspot/limits/global/speed-history?minutes=${minutes}`),
    refetchInterval: refetchIntervalForWindow(minutes),
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
    refetchInterval: 1000,
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

export function useDeviceCreditHistory(mac: string) {
  return useQuery<HotspotCreditHistoryEntry[]>({
    queryKey: ["hotspot", "devices", mac, "credit", "history"],
    queryFn: () => api.get<HotspotCreditHistoryEntry[]>(`/hotspot/devices/${encodeURIComponent(mac)}/credit/history`),
    enabled: !!mac,
  });
}

// Detalhe de consumo de uma sessao especifica - busca o trace bruto no
// Mongo (services/backend/hotspot_sessions.go) filtrado pela janela de
// tempo daquela sessao. So habilitada quando o modal de detalhe esta
// aberto (sessionId != null), ver DeviceMovementsCard.tsx.
export function useSessionConsumption(mac: string, sessionId: number | null) {
  return useQuery<HotspotSessionConsumptionEntry[]>({
    queryKey: ["hotspot", "devices", mac, "sessions", sessionId, "consumption"],
    queryFn: () =>
      api.get<HotspotSessionConsumptionEntry[]>(
        `/hotspot/devices/${encodeURIComponent(mac)}/sessions/${sessionId}/consumption`,
      ),
    enabled: !!mac && sessionId !== null,
  });
}
