import type { ConfigForm } from "@/components/hotspot/hotspot-schema";
import { generateRandomWifiPassword } from "@/components/hotspot/generate-password";

interface WifiInterfaceOption {
  name: string;
}

// Deriva os valores iniciais do formulario de configuracao do hotspot a
// partir do que ja esta salvo (GET /api/hotspot/config) - usado tanto
// pela tela principal (pages/Hotspot.tsx) quanto pelo assistente de
// configuracao inicial (setup/SetupHotspotStep.tsx), que preenchem o
// mesmo formulario em momentos diferentes do fluxo. ssidFallback e o
// unico ponto onde os dois divergiam ("Bindnet" no assistente, vazio na
// tela principal).
export function hotspotConfigDefaults(
  config: Record<string, string> | undefined,
  wifiInterfaces: WifiInterfaceOption[],
  ssidFallback = "",
): ConfigForm {
  const data = config ?? {};
  const suggestedInterface = data.WIFI_INTERFACE || (wifiInterfaces.length === 1 ? wifiInterfaces[0].name : "");
  return {
    WIFI_SSID: data.WIFI_SSID || ssidFallback,
    WIFI_PASSWORD: data.WIFI_PASSWORD || generateRandomWifiPassword(),
    WIFI_OPEN: data.WIFI_OPEN === "true" ? "true" : "false",
    WIFI_INTERFACE: suggestedInterface,
    INTERNET_INTERFACE: data.INTERNET_INTERFACE || "auto",
    WIFI_COUNTRY: data.WIFI_COUNTRY ?? "ST",
    WIFI_CHANNEL: data.WIFI_CHANNEL ?? "auto",
    WIFI_FREQ_BAND: data.WIFI_FREQ_BAND ?? "auto",
    WIFI_CHANNEL_CANDIDATES: data.WIFI_CHANNEL_CANDIDATES ?? "",
    HOTSPOT_GATEWAY: data.HOTSPOT_GATEWAY || "192.168.12.1",
    HOTSPOT_CIDR: data.HOTSPOT_CIDR || "192.168.12.0/24",
    HOTSPOT_DNS_FALLBACKS: data.HOTSPOT_DNS_FALLBACKS ?? "1.1.1.1,8.8.8.8",
    BINDNET_UPLINK_INTERFACE: data.BINDNET_UPLINK_INTERFACE || "bn-uplink",
    UPLINK_MONITOR_INTERVAL: data.UPLINK_MONITOR_INTERVAL || "10",
  };
}
