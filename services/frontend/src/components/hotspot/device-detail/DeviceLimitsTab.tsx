import { Button } from "@/components/ui/button";
import { HotspotLimitsForm } from "@/components/hotspot/HotspotLimitsForm";
import { HotspotQuotaProgress } from "@/components/hotspot/HotspotQuotaProgress";
import { useDeviceLimits, useDeviceTraffic } from "@/components/hotspot/useHotspotQueries";
import { useDeviceLimitsMutation } from "@/components/hotspot/useHotspotLimitMutations";

export function DeviceLimitsTab({ mac }: { mac: string }) {
  const limits = useDeviceLimits(mac);
  const traffic = useDeviceTraffic(mac);
  const { save, remove } = useDeviceLimitsMutation(mac);

  if (!limits.data) {
    return <div className="h-48 animate-pulse rounded-lg border bg-muted/30" />;
  }

  return (
    <div className="space-y-6">
      {traffic.data && <HotspotQuotaProgress traffic={traffic.data} />}
      <HotspotLimitsForm value={limits.data} onSubmit={(value) => save.mutate(value)} pending={save.isPending} />
      <Button variant="outline" onClick={() => remove.mutate()} disabled={remove.isPending}>
        Remover limite (voltar ao teto global)
      </Button>
    </div>
  );
}
