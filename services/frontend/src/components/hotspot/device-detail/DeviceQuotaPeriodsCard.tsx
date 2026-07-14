import { HotspotQuotaPeriodBars } from "@/components/hotspot/HotspotQuotaPeriodBars";
import { useDeviceQuotaPeriods } from "@/components/hotspot/useHotspotQueries";
import { useResetDeviceQuotaPeriod } from "@/components/hotspot/useHotspotDeviceQuotaMutations";
import type { QuotaPeriod } from "@/components/hotspot/hotspot-limits-types";

// So renderizado pelo DeviceLimitsTab quando o LimitType efetivo do
// dispositivo e "quota" - busca os ate 3 periodos configurados e liga o
// botao de reset por periodo (requisito: um botao por cota, nao um
// unico que zera tudo).
export function DeviceQuotaPeriodsCard({ mac }: { mac: string }) {
  const periods = useDeviceQuotaPeriods(mac);
  const reset = useResetDeviceQuotaPeriod(mac);

  if (!periods.data) {
    return <div className="h-32 animate-pulse rounded-lg border bg-muted/30" />;
  }

  return (
    <HotspotQuotaPeriodBars
      periods={periods.data}
      onReset={(period: QuotaPeriod) => reset.mutate(period)}
      resetPending={reset.isPending ? (reset.variables as QuotaPeriod) ?? null : null}
    />
  );
}
