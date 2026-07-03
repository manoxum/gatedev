import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { HotspotLimitsForm } from "@/components/hotspot/HotspotLimitsForm";
import { useGlobalLimits, useGlobalTraffic } from "@/components/hotspot/useHotspotQueries";
import { useGlobalLimitsMutation } from "@/components/hotspot/useHotspotLimitMutations";
import { HotspotQuotaProgress } from "@/components/hotspot/HotspotQuotaProgress";

// Card de limite global (todo o hotspot), na tela principal do hotspot -
// mesmo HotspotLimitsForm reusado pela página de detalhe de dispositivo.
export function GlobalLimitsCard() {
  const limits = useGlobalLimits();
  const traffic = useGlobalTraffic();
  const { save } = useGlobalLimitsMutation();

  if (!limits.data) {
    return <div className="h-48 animate-pulse rounded-lg border bg-muted/30" />;
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Limite global do hotspot</CardTitle>
      </CardHeader>
      <CardContent className="space-y-6">
        {traffic.data && <HotspotQuotaProgress traffic={traffic.data} />}
        <HotspotLimitsForm value={limits.data} onSubmit={(value) => save.mutate(value)} pending={save.isPending} />
      </CardContent>
    </Card>
  );
}
