import { lazy, Suspense } from "react";
import { Flag, Globe, Hash, Play, RefreshCw, Router, Settings2, Square, Waves, Wifi, type LucideIcon } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { HotspotWifiQr } from "@/components/hotspot/HotspotWifiQr";
import {
  HotspotInterfaceQuickSwitch,
  type InterfaceQuickSwitchOption,
} from "@/components/hotspot/HotspotInterfaceQuickSwitch";

// Carregado sob demanda: puxa o ApexCharts (~170KB gzip), que so faz
// sentido pagar em paginas de hotspot, nao no bundle principal
// carregado em toda rota do painel.
const HotspotGlobalSpeedPanel = lazy(() =>
  import("@/components/hotspot/HotspotGlobalSpeedPanel").then((m) => ({ default: m.HotspotGlobalSpeedPanel })),
);

interface HotspotSummaryCardProps {
  config: Record<string, string>;
  running: boolean;
  startPending: boolean;
  stopPending: boolean;
  recoverPending: boolean;
  onStart: () => void;
  onStop: () => void;
  onRecover: () => void;
  onEdit: () => void;
  currentBand?: string;
  currentChannel?: string;
  currentInternetInterface?: string;
  wifiInterfaceOptions: InterfaceQuickSwitchOption[];
  internetInterfaceOptions: InterfaceQuickSwitchOption[];
  onQuickSwitchInterface: (field: "WIFI_INTERFACE" | "INTERNET_INTERFACE", value: string) => void;
  quickSwitchPending?: boolean;
}

// autoValue mostra "auto (valor real)" quando o campo esta configurado
// como "auto" e ja existe um valor real resolvido pelo worker (vindo do
// status do hotspot rodando); caso contrario mostra so o valor
// configurado, sem inventar um "real" quando o hotspot esta parado.
function autoValue(configured: string | undefined, resolved: string | undefined): string {
  if (configured === "auto" && resolved) {
    return `auto (${resolved})`;
  }
  return configured ?? "";
}

// Compacto de proposito (icone menor, menos padding, texto menor): o
// espaco poupado aqui sobra pro painel geral de velocidade
// (HotspotGlobalSpeedPanel) a direita, no mesmo CardContent.
function ConfigItem({ icon: Icon, label, value }: { icon: LucideIcon; label: string; value: string }) {
  return (
    <div className="flex items-center gap-2 rounded-lg border border-border/60 bg-muted/30 px-2 py-1.5 transition-colors hover:bg-muted/60">
      <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
        <Icon className="h-3.5 w-3.5" />
      </div>
      <div className="min-w-0">
        <p className="text-[10px] leading-tight text-muted-foreground">{label}</p>
        <p className="truncate text-xs font-semibold leading-tight">{value || "—"}</p>
      </div>
    </div>
  );
}

// Mostra a configuração atualmente aplicada (a que está em vigor no hotspot,
// não a do formulário ainda não salvo) e um QR para conectar direto pelo celular.
export function HotspotSummaryCard({
  config,
  running,
  startPending,
  stopPending,
  recoverPending,
  onStart,
  onStop,
  onRecover,
  onEdit,
  currentBand,
  currentChannel,
  currentInternetInterface,
  wifiInterfaceOptions,
  internetInterfaceOptions,
  onQuickSwitchInterface,
  quickSwitchPending,
}: HotspotSummaryCardProps) {
  const ssid = config.WIFI_SSID ?? "";
  const password = config.WIFI_PASSWORD ?? "";

  return (
    <Card>
      <CardHeader className="flex flex-col gap-4 border-b border-border/60 pb-5 sm:flex-row sm:items-start sm:justify-between space-y-0">
        <div className="flex items-center gap-3">
          <div>
            <CardTitle>Configuração atual</CardTitle>
            <CardDescription>Valores em vigor no hotspot neste momento.</CardDescription>
          </div>
          <Badge variant={running ? "success" : "secondary"}>{running ? "ligado" : "desligado"}</Badge>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button size="sm" onClick={onStart} disabled={running || startPending || recoverPending}>
            <Play className="h-4 w-4" />
            Iniciar
          </Button>
          <Button size="sm" variant="destructive" onClick={onStop} disabled={!running || stopPending || recoverPending}>
            <Square className="h-4 w-4" />
            Parar
          </Button>
          <Button
            size="sm"
            variant="secondary"
            onClick={onRecover}
            disabled={recoverPending || startPending || stopPending}
          >
            <RefreshCw className={recoverPending ? "h-4 w-4 animate-spin" : "h-4 w-4"} />
            Recuperar Wi-Fi
          </Button>
          <Button variant="outline" size="sm" onClick={onEdit}>
            <Settings2 className="h-4 w-4" />
            Alterar configuração
          </Button>
        </div>
      </CardHeader>
      <CardContent className="flex flex-col gap-4 pt-5 lg:flex-row lg:items-stretch lg:justify-between">
        {ssid && password && <HotspotWifiQr ssid={ssid} password={password} />}
        <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:w-auto lg:shrink-0">
          <ConfigItem icon={Wifi} label="SSID" value={ssid} />
          <HotspotInterfaceQuickSwitch
            icon={Router}
            label="Interface Wi-Fi"
            value={config.WIFI_INTERFACE ?? ""}
            options={wifiInterfaceOptions}
            onChange={(value) => onQuickSwitchInterface("WIFI_INTERFACE", value)}
            disabled={quickSwitchPending}
          />
          <HotspotInterfaceQuickSwitch
            icon={Globe}
            label="Interface de internet"
            value={config.INTERNET_INTERFACE ?? ""}
            displayValue={autoValue(config.INTERNET_INTERFACE, currentInternetInterface)}
            options={internetInterfaceOptions}
            onChange={(value) => onQuickSwitchInterface("INTERNET_INTERFACE", value)}
            disabled={quickSwitchPending}
          />
          <ConfigItem icon={Flag} label="País" value={config.WIFI_COUNTRY ?? ""} />
          <ConfigItem
            icon={Waves}
            label="Banda"
            value={autoValue(config.WIFI_FREQ_BAND, currentBand ? `${currentBand}GHz` : undefined)}
          />
          <ConfigItem icon={Hash} label="Canal" value={autoValue(config.WIFI_CHANNEL, currentChannel)} />
        </div>
        <div className="flex min-w-0 flex-1 items-stretch">
          <Suspense fallback={<div className="h-full min-h-[140px] w-full animate-pulse rounded-xl bg-muted/30" />}>
            <HotspotGlobalSpeedPanel />
          </Suspense>
        </div>
      </CardContent>
    </Card>
  );
}
