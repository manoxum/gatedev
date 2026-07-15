import { lazy, Suspense, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { ArrowLeft, CreditCard, LayoutGrid, Sliders } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { EmptyState } from "@/components/bindnets/EmptyState";
import { useHotspotQueries } from "@/components/hotspot/useHotspotQueries";
import { DeviceOverviewTab } from "@/components/hotspot/device-detail/DeviceOverviewTab";
import { DeviceLimitsTab } from "@/components/hotspot/device-detail/DeviceLimitsTab";
import { DeviceCreditCard } from "@/components/hotspot/device-detail/DeviceCreditCard";
import { DeviceMovementsCard } from "@/components/hotspot/device-detail/DeviceMovementsCard";
import { DeviceSpeedGaugeCard } from "@/components/hotspot/device-detail/DeviceSpeedGaugeCard";
import { SPEED_CHART_DEFAULT_WINDOW_MINUTES } from "@/components/hotspot/device-detail/speed-chart-windows";
import { usePageHeader } from "@/hooks/usePageHeader";
import { useUrlTab } from "@/hooks/useUrlTab";
import { blockStatusLabel, type HotspotClient } from "@/components/hotspot/HotspotClientsCard";
import type { ByteNature } from "@/components/hotspot/hotspot-limits-types";

// Carregado sob demanda: puxa o ApexCharts (~170KB gzip), que so faz
// sentido pagar em paginas de hotspot, nao no bundle principal
// carregado em toda rota do painel.
const DeviceSpeedChart = lazy(() =>
  import("@/components/hotspot/device-detail/DeviceSpeedChart").then((m) => ({ default: m.DeviceSpeedChart })),
);

// Dispositivo desconectado no momento nao aparece em `clients` (lista
// ao vivo), so em `knownDevices` (todo MAC que ja apareceu alguma vez -
// ver HotspotKnownDevicesCard.tsx) - monta um HotspotClient equivalente
// a partir dali (sem ip/hostname/perfil ao vivo) pra essa pagina
// continuar funcionando mesmo offline, em vez de so mostrar "nao
// encontrado".
function knownDeviceAsClient(
  known: NonNullable<ReturnType<typeof useHotspotQueries>["knownDevices"]["data"]>[number],
): HotspotClient {
  return {
    mac: known.mac,
    ip: "",
    hostname: "",
    vendor: known.vendor,
    deviceName: known.deviceName,
    osName: known.osName,
    alias: known.alias,
    blocked: known.blocked,
    blockReason: known.blockReason,
  };
}

export function HotspotDeviceDetailPage() {
  const { mac: macParam } = useParams();
  const navigate = useNavigate();
  const mac = macParam ? decodeURIComponent(macParam) : "";
  // Levantado pro pai (nao um useState local em DeviceSpeedChart.tsx)
  // pra o velocimetro ao lado (DeviceSpeedGaugeCard.tsx) mostrar o
  // valor na mesma unidade escolhida no grafico.
  const [windowMinutes, setWindowMinutes] = useState(SPEED_CHART_DEFAULT_WINDOW_MINUTES);
  const [unitNature, setUnitNature] = useState<ByteNature>("bit");
  const [tab, setTab] = useUrlTab("overview");

  const { clients, knownDevices } = useHotspotQueries();
  const liveClient = clients.data?.find((candidate) => candidate.mac === mac);
  const knownDevice = knownDevices.data?.find((candidate) => candidate.mac === mac);
  const client = liveClient ?? (knownDevice ? knownDeviceAsClient(knownDevice) : undefined);
  const online = !!liveClient;
  const loading = clients.isLoading || knownDevices.isLoading;

  usePageHeader({ title: client?.alias || client?.deviceName || client?.vendor || mac, description: client?.ip });

  if (!loading && !client) {
    return (
      <div className="space-y-4">
        <Button variant="ghost" onClick={() => navigate("/hotspot")}>
          <ArrowLeft className="h-4 w-4" />
          Voltar
        </Button>
        <EmptyState label="Dispositivo não encontrado (nunca se conectou ao hotspot)." />
      </div>
    );
  }

  if (!client) {
    return <div className="h-80 animate-pulse rounded-lg border bg-muted/30" />;
  }

  return (
    <div className="space-y-6">
      <div className="space-y-3">
        <Button variant="ghost" className="px-0" onClick={() => navigate("/hotspot")}>
          <ArrowLeft className="h-4 w-4" />
          Hotspot
        </Button>
        <div className="flex flex-wrap items-center gap-3">
          <div>
            <h1 className="text-2xl font-semibold">{client.alias || client.deviceName || client.vendor || "Dispositivo"}</h1>
            <p className="font-mono text-sm text-muted-foreground">{client.mac}</p>
          </div>
          {(() => {
            const status = blockStatusLabel(client, online);
            return <Badge variant={status.variant}>{status.label}</Badge>;
          })()}
        </div>
      </div>

      <Tabs value={tab} onValueChange={setTab}>
        <TabsList className="grid h-auto w-full grid-cols-3 sm:inline-grid sm:w-auto">
          <TabsTrigger value="overview">
            <LayoutGrid className="h-4 w-4" />
            Visão geral
          </TabsTrigger>
          <TabsTrigger value="limits">
            <Sliders className="h-4 w-4" />
            Limites
          </TabsTrigger>
          <TabsTrigger value="credit">
            <CreditCard className="h-4 w-4" />
            Movimentações
          </TabsTrigger>
        </TabsList>
        <TabsContent value="overview" className="space-y-4">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-stretch">
            <div className="lg:flex-1">
              <Suspense fallback={<div className="h-64 animate-pulse rounded-lg border bg-muted/30" />}>
                <DeviceSpeedChart
                  mac={client.mac}
                  windowMinutes={windowMinutes}
                  onWindowChange={setWindowMinutes}
                  unitNature={unitNature}
                  onUnitChange={setUnitNature}
                />
              </Suspense>
            </div>
            <DeviceSpeedGaugeCard mac={client.mac} unitNature={unitNature} />
          </div>
          <DeviceOverviewTab client={client} online={online} />
        </TabsContent>
        <TabsContent value="limits">
          <DeviceLimitsTab mac={client.mac} />
        </TabsContent>
        <TabsContent value="credit" className="space-y-4">
          <DeviceCreditCard mac={client.mac} />
          <DeviceMovementsCard mac={client.mac} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
