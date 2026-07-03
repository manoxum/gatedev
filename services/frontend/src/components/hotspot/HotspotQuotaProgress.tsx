import { Progress } from "@/components/ui/progress";
import { bytesToGB } from "@/components/hotspot/hotspot-limits-types";
import type { HotspotTraffic } from "@/components/hotspot/hotspot-limits-types";

const periodLabels: Record<string, string> = {
  daily: "diária",
  weekly: "semanal",
  monthly: "mensal",
};

// Barra de progresso de cota (compartilhada entre o limite global e o
// limite por dispositivo) - se não há cota configurada, mostra só o
// total trafegado no período sem barra.
export function HotspotQuotaProgress({ traffic }: { traffic: HotspotTraffic }) {
  const usedBytes = traffic.downloadBytes + traffic.uploadBytes;
  const usedGB = bytesToGB(usedBytes).toFixed(2);

  if (!traffic.quotaBytes) {
    return (
      <p className="text-sm text-muted-foreground">
        {usedGB}GB trafegados no período corrente (sem cota configurada).
      </p>
    );
  }

  const quotaGB = bytesToGB(traffic.quotaBytes).toFixed(2);
  const percent = (usedBytes / traffic.quotaBytes) * 100;
  const periodLabel = traffic.quotaPeriod ? periodLabels[traffic.quotaPeriod] : "";

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between text-sm">
        <span className="text-muted-foreground">Cota {periodLabel}</span>
        <span className={traffic.throttled ? "font-medium text-destructive" : "font-medium"}>
          {usedGB}GB / {quotaGB}GB
        </span>
      </div>
      <Progress value={percent} />
      {traffic.throttled && <p className="text-xs text-destructive">Cota estourada - trafegando em modo throttle.</p>}
    </div>
  );
}
