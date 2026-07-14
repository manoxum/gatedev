import { Badge } from "@/components/ui/badge";
import { Card, CardContent } from "@/components/ui/card";
import { SpeedGauge } from "@/components/ui/speed-gauge";
import { HotspotQuotaPeriodBars } from "@/components/hotspot/HotspotQuotaPeriodBars";
import { LIMIT_TYPE_LABELS, rateToBps, toBitsPerSecond, type ByteNature } from "@/components/hotspot/hotspot-limits-types";
import { useDeviceLimits, useDeviceQuotaPeriods, useDeviceStats } from "@/components/hotspot/useHotspotQueries";

// Velocimetro "agora" do dispositivo (poll de 2.5s, useDeviceStats),
// ao lado do grafico de tendencia (DeviceSpeedChart.tsx) na aba
// "Visao geral" - card separado de proposito (nao embutido no
// cabecalho do grafico) para os dois ficarem lado a lado em telas
// largas. Teto do arco usa o limite configurado do dispositivo, quando
// houver; unitNature vem do mesmo estado do grafico ao lado (levantado
// pro pai, HotspotDeviceDetail.tsx) pra nao mostrar bits aqui e bytes
// la.
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

  return (
    <Card className="flex h-full flex-col lg:w-auto lg:shrink-0">
      <CardContent className="flex flex-1 flex-col justify-center gap-4 pt-6">
        <div className="flex items-center justify-center gap-4">
          <SpeedGauge
            valueBps={toBitsPerSecond(stats.data?.downloadBps ?? 0)}
            maxBps={limits.data ? rateToBps(limits.data.downloadRateValue, limits.data.downloadRateUnit) : null}
            label="Download"
            size="lg"
            unitNature={unitNature}
          />
          <SpeedGauge
            valueBps={toBitsPerSecond(stats.data?.uploadBps ?? 0)}
            maxBps={limits.data ? rateToBps(limits.data.uploadRateValue, limits.data.uploadRateUnit) : null}
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
