import { useNavigate, useParams } from "react-router-dom";
import { ArrowLeft, CreditCard, LayoutGrid, Sliders } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { EmptyState } from "@/components/bindnets/EmptyState";
import { useHotspotQueries } from "@/components/hotspot/useHotspotQueries";
import { DeviceOverviewTab } from "@/components/hotspot/device-detail/DeviceOverviewTab";
import { DeviceLimitsTab } from "@/components/hotspot/device-detail/DeviceLimitsTab";
import { DeviceCreditCard } from "@/components/hotspot/device-detail/DeviceCreditCard";
import { DeviceCreditHistoryCard } from "@/components/hotspot/device-detail/DeviceCreditHistoryCard";
import { DeviceSpeedCard } from "@/components/hotspot/device-detail/DeviceSpeedCard";
import { usePageHeader } from "@/hooks/usePageHeader";

export function HotspotDeviceDetailPage() {
  const { mac: macParam } = useParams();
  const navigate = useNavigate();
  const mac = macParam ? decodeURIComponent(macParam) : "";

  const { clients } = useHotspotQueries();
  const client = clients.data?.find((candidate) => candidate.mac === mac);

  usePageHeader({ title: client?.alias || client?.deviceName || client?.vendor || mac, description: client?.ip });

  if (!clients.isLoading && !client) {
    return (
      <div className="space-y-4">
        <Button variant="ghost" onClick={() => navigate("/hotspot")}>
          <ArrowLeft className="h-4 w-4" />
          Voltar
        </Button>
        <EmptyState label="Dispositivo não encontrado (pode ter se desconectado)." />
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
        <div>
          <h1 className="text-2xl font-semibold">{client.alias || client.deviceName || client.vendor || "Dispositivo"}</h1>
          <p className="font-mono text-sm text-muted-foreground">{client.mac}</p>
        </div>
      </div>

      <Tabs defaultValue="overview">
        <TabsList>
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
            Crédito
          </TabsTrigger>
        </TabsList>
        <TabsContent value="overview" className="space-y-4">
          <DeviceSpeedCard mac={client.mac} />
          <DeviceOverviewTab client={client} />
        </TabsContent>
        <TabsContent value="limits">
          <DeviceLimitsTab mac={client.mac} />
        </TabsContent>
        <TabsContent value="credit" className="space-y-4">
          <DeviceCreditCard mac={client.mac} />
          <DeviceCreditHistoryCard mac={client.mac} />
        </TabsContent>
      </Tabs>
    </div>
  );
}
