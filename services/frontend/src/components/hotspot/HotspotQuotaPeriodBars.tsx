import { Progress } from "@/components/ui/progress";
import { Button } from "@/components/ui/button";
import { formatQuotaValue } from "@/components/hotspot/hotspot-limits-types";
import type { HotspotDeviceQuotaPeriodUsage, QuotaPeriod } from "@/components/hotspot/hotspot-limits-types";

const periodLabels: Record<QuotaPeriod, string> = {
  daily: "diária",
  weekly: "semanal",
  monthly: "mensal",
};

interface HotspotQuotaPeriodBarsProps {
  periods: HotspotDeviceQuotaPeriodUsage[];
  // onReset ausente = view de auto-serviço (portal do dispositivo, sem
  // acao de admin) - so desenha as barras, sem botao.
  onReset?: (period: QuotaPeriod) => void;
  resetPending?: QuotaPeriod | null;
}

// Uma barra de progresso + botao "Resetar" por periodo configurado
// (nao um botao unico que zera tudo) - reusada pelo detalhe de
// dispositivo (com onReset) e pelo portal de autoatendimento (sem
// onReset).
export function HotspotQuotaPeriodBars({ periods, onReset, resetPending }: HotspotQuotaPeriodBarsProps) {
  if (periods.length === 0) {
    return <p className="text-sm text-muted-foreground">Nenhuma cota configurada.</p>;
  }

  return (
    <div className="space-y-4">
      {periods.map((period) => {
        const usedBytes = period.downloadBytes + period.uploadBytes;
        const remainingBytes = Math.max(period.quotaBytes - usedBytes, 0);
        const used = formatQuotaValue(usedBytes, period.quotaUnit);
        const quota = formatQuotaValue(period.quotaBytes, period.quotaUnit);
        const remaining = formatQuotaValue(remainingBytes, period.quotaUnit);
        const percent = period.quotaBytes > 0 ? (usedBytes / period.quotaBytes) * 100 : 0;
        return (
          <div key={period.periodType} className="space-y-2">
            <div className="flex items-center justify-between text-sm">
              <span className="text-muted-foreground">Cota {periodLabels[period.periodType]}</span>
              <span className={period.blocked ? "font-medium text-destructive" : "font-medium"}>
                {used} / {quota}
              </span>
            </div>
            <Progress value={Math.min(percent, 100)} />
            <p className="text-xs text-muted-foreground">
              {period.blocked ? `Restante: ${formatQuotaValue(0, period.quotaUnit)}` : `Restante: ${remaining}`}
            </p>
            <div className="flex items-center justify-between gap-2">
              {period.blocked ? (
                <p className="text-xs text-destructive">Cota estourada - tráfego bloqueado.</p>
              ) : (
                <span />
              )}
              {onReset && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => onReset(period.periodType)}
                  disabled={resetPending === period.periodType}
                >
                  Resetar
                </Button>
              )}
            </div>
          </div>
        );
      })}
    </div>
  );
}
