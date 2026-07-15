import { z } from "zod";

// WIFI_OPEN="true" cria um hotspot livre, sem autenticacao (ver
// try_create_ap em services/worker/hotspot/entrypoint.sh - create_ap so
// aplica WPA2 quando recebe uma passphrase). Nesse caso WIFI_PASSWORD
// deixa de ser obrigatória - por isso a checagem de tamanho mínimo vira
// um .refine() em vez de min(8) direto no campo.
export const configSchema = z
  .object({
    WIFI_SSID: z.string().min(1, "Informe o SSID"),
    WIFI_PASSWORD: z.string(),
    WIFI_OPEN: z.enum(["true", "false"]),
    WIFI_INTERFACE: z.string().min(1, "Selecione a interface Wi-Fi"),
    INTERNET_INTERFACE: z.string().min(1, "Selecione a interface de internet"),
    WIFI_COUNTRY: z.string().min(2).max(2),
    WIFI_CHANNEL: z.string().min(1),
    WIFI_FREQ_BAND: z.string().min(1),
    WIFI_CHANNEL_CANDIDATES: z.string(),
    HOTSPOT_GATEWAY: z.string().min(1, "Informe o gateway do hotspot"),
    HOTSPOT_CIDR: z.string().min(1, "Informe a faixa CIDR do hotspot"),
    HOTSPOT_DNS_FALLBACKS: z.string(),
    BINDNET_UPLINK_INTERFACE: z.string().min(1, "Informe o uplink virtual"),
    UPLINK_MONITOR_INTERVAL: z.string().min(1, "Informe o intervalo"),
  })
  .refine((data) => data.WIFI_OPEN === "true" || data.WIFI_PASSWORD.length >= 8, {
    message: "Mínimo de 8 caracteres (WPA2), a menos que o hotspot seja livre (sem senha)",
    path: ["WIFI_PASSWORD"],
  });
export type ConfigForm = z.infer<typeof configSchema>;
