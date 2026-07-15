import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { SpeedGauge } from "@/components/ui/speed-gauge";
import { HotspotQuotaPeriodBars } from "@/components/hotspot/HotspotQuotaPeriodBars";
import {
  LIMIT_TYPE_LABELS,
  rateToBps,
  toBitsPerSecond,
  type ByteNature,
  type RateUnit,
} from "@/components/hotspot/hotspot-limits-types";
import { useDeviceLimits, useDeviceQuotaPeriods, useDeviceSpeedHistory, useDeviceStats } from "@/components/hotspot/useHotspotQueries";

const GAUGE_MAX_WINDOW_MINUTES = 1;

// gaugeMaxBps decide o teto do velocimetro numa unica regra: taxa
// EFETIVA configurada (direta no dispositivo ou herdada do perfil,
// useDeviceLimits ja resolve isso) quando existir; sem taxa nenhuma,
// cai na MESMA estrategia do painel global (HotspotGlobalSpeedPanel.tsx) -
// 2x a media do ultimo minuto, undefined (autoescala generico do
// SpeedGauge) so enquanto ainda nao ha amostra nenhuma dessa janela.
function gaugeMaxBps(rateValue: number | null, rateUnit: RateUnit, avgBps: number | undefined): number | undefined {
  if (rateValue) return rateToBps(rateValue, rateUnit) ?? undefined;
  return avgBps !== undefined ? toBitsPerSecond(avgBps) * 2 : undefined;
}

// Velocimetro "agora" do dispositivo (poll de 2.5s, useDeviceStats),
// ao lado do grafico de tendencia (DeviceSpeedChart.tsx) na aba
// "Visao geral" - card separado de proposito (nao embutido no
// cabecalho do grafico) para os dois ficarem lado a lado em telas
// largas. Teto do arco: ver gaugeMaxBps acima (taxa efetiva do
// dispositivo/perfil quando houver, senao a mesma estrategia do
// velocimetro global); unitNature vem do mesmo estado do grafico ao
// lado (levantado pro pai, HotspotDeviceDetail.tsx) pra nao mostrar
// bits aqui e bytes la.
//
// O rodape com perfil/limite/cota (abaixo dos velocimetros) nao e so
// enchimento: os dois velocimetros sozinhos deixavam uma sobra de
// espaco vazio abaixo - o resumo do limite efetivo (mesmos dados que
// useDeviceLimits ja busca aqui, sem chamada nova) explica de quebra
// por que os velocimetros tem o teto que tem, e reaproveita
// HotspotQuotaPeriodBars (mesmo componente da aba Limites) quando o
// tipo e "quota". max-h+overflow no bloco de cota: com os 3 periodos
// (diario/semanal/mensal) configurados ao mesmo tempo o conteudo podia
// crescer bem mais alto que o card do grafico ao lado - o teto+scroll
// interno evita isso, o que permite o card inteiro esticar ate a
// mesma altura do irmao (h-full, ver items-stretch no pai) sem que a
// lista de periodos estoure por baixo.
export function DeviceSpeedGaugeCard({ mac, unitNature }: { mac: string; unitNature: ByteNature }) {
  const stats = useDeviceStats(mac);
  const limits = useDeviceLimits(mac);
  const isQuota = limits.data?.limitType === "quota";
  const quotaPeriods = useDeviceQuotaPeriods(isQuota ? mac : "");

  const needsAutoScale = !!limits.data && (!limits.data.downloadRateValue || !limits.data.uploadRateValue);
  const gaugeMaxHistory = useDeviceSpeedHistory(needsAutoScale ? mac : "", GAUGE_MAX_WINDOW_MINUTES);
  const gaugeMaxSamples = gaugeMaxHistory.data ?? [];
  const avgDownloadBps =
    gaugeMaxSamples.length > 0
      ? gaugeMaxSamples.reduce((sum, s) => sum + s.downloadBps, 0) / gaugeMaxSamples.length
      : undefined;
  const avgUploadBps =
    gaugeMaxSamples.length > 0
      ? gaugeMaxSamples.reduce((sum, s) => sum + s.uploadBps, 0) / gaugeMaxSamples.length
      : undefined;

  return (
    <Card className="flex h-full flex-col lg:w-auto lg:shrink-0">
      <CardContent className="flex flex-1 flex-col justify-center gap-4 pt-6">
        <div className="flex items-center justify-center gap-4">
          <SpeedGauge
            valueBps={toBitsPerSecond(stats.data?.downloadBps ?? 0)}
            maxBps={
              limits.data
                ? gaugeMaxBps(limits.data.downloadRateValue, limits.data.downloadRateUnit, avgDownloadBps)
                : undefined
            }
            label="Download"
            size="lg"
            unitNature={unitNature}
          />
          <SpeedGauge
            valueBps={toBitsPerSecond(stats.data?.uploadBps ?? 0)}
            maxBps={
              limits.data
                ? gaugeMaxBps(limits.data.uploadRateValue, limits.data.uploadRateUnit, avgUploadBps)
                : undefined
            }
            label="Upload"
            size="lg"
            unitNature={unitNature}
          />
        </div>
        {limits.data && (
          <div className="flex flex-col gap-3 border-t border-border/60 pt-3">
            <div className="flex items-center justify-between gap-2 text-sm">
              <span className="truncate text-muted-foreground">
                Perfil <span className="font-medium text-foreground">{limits.data.profileName}</span>
              </span>
              <Badge variant="secondary" className="shrink-0">
                {LIMIT_TYPE_LABELS[limits.data.limitType]}
              </Badge>
            </div>
            {isQuota && quotaPeriods.data && (
              <div className="max-h-56 overflow-y-auto pr-1">
                <HotspotQuotaPeriodBars periods={quotaPeriods.data} />
              </div>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
