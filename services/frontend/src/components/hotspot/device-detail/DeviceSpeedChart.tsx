import { useMemo } from "react";
import Chart from "react-apexcharts";
import type { ApexOptions } from "apexcharts";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { SelectNative } from "@/components/ui/select-native";
import { cn } from "@/lib/utils";
import { useApexChartColors } from "@/components/hotspot/apex-chart-theme";
import {
  formatSpeedNow,
  pickByteScale,
  rateToBps,
  smoothSpeedSamples,
  toBitsPerSecond,
  type ByteNature,
} from "@/components/hotspot/hotspot-limits-types";
import { useDeviceLimits, useDeviceSpeedHistory, useDeviceStats } from "@/components/hotspot/useHotspotQueries";
import { SPEED_CHART_WINDOWS } from "@/components/hotspot/device-detail/speed-chart-windows";

function WindowSelect({ windowMinutes, onChange }: { windowMinutes: number; onChange: (minutes: number) => void }) {
  return (
    <SelectNative
      className="h-8 w-28 text-xs"
      value={windowMinutes}
      onChange={(e) => onChange(Number(e.target.value))}
    >
      {SPEED_CHART_WINDOWS.map((w) => (
        <option key={w.minutes} value={w.minutes}>
          {w.label}
        </option>
      ))}
    </SelectNative>
  );
}

const UNIT_NATURES: { nature: ByteNature; label: string }[] = [
  { nature: "bit", label: "bits" },
  { nature: "byte", label: "bytes" },
];

function UnitSelect({ nature, onChange }: { nature: ByteNature; onChange: (nature: ByteNature) => void }) {
  return (
    <SelectNative
      className="h-8 w-24 text-xs"
      value={nature}
      onChange={(e) => onChange(e.target.value as ByteNature)}
    >
      {UNIT_NATURES.map((u) => (
        <option key={u.nature} value={u.nature}>
          {u.label}
        </option>
      ))}
    </SelectNative>
  );
}

// SpeedChartHeader reune tudo numa unica linha (com flex-wrap so pra
// telas estreitas): titulo, leitura "agora" de Download/Upload (poll de
// 2.5s, useDeviceStats) e os dois seletores (unidade, janela) - ambos
// controlados pelo pai (HotspotDeviceDetail.tsx) pra o velocimetro ao
// lado (DeviceSpeedGaugeCard.tsx) tambem respeitar a mesma unidade. O
// valor instantaneo fica destacado em vermelho quando ultrapassa o
// limite configurado do dispositivo - sempre visivel, inclusive no
// estado "sem amostras" abaixo.
function SpeedChartHeader({
  mac,
  windowMinutes,
  onWindowChange,
  unitNature,
  onUnitChange,
}: {
  mac: string;
  windowMinutes: number;
  onWindowChange: (minutes: number) => void;
  unitNature: ByteNature;
  onUnitChange: (nature: ByteNature) => void;
}) {
  const stats = useDeviceStats(mac);
  const limits = useDeviceLimits(mac);
  const colors = useApexChartColors();

  const downloadBps = stats.data?.downloadBps ?? 0;
  const uploadBps = stats.data?.uploadBps ?? 0;
  const maxDownloadBps = limits.data ? rateToBps(limits.data.downloadRateValue, limits.data.downloadRateUnit) : null;
  const maxUploadBps = limits.data ? rateToBps(limits.data.uploadRateValue, limits.data.uploadRateUnit) : null;
  const overDownload = maxDownloadBps != null && toBitsPerSecond(downloadBps) > maxDownloadBps;
  const overUpload = maxUploadBps != null && toBitsPerSecond(uploadBps) > maxUploadBps;

  return (
    <CardHeader className="flex flex-row flex-wrap items-center gap-x-4 gap-y-1.5 space-y-0">
      <CardTitle className="text-base">Velocidade</CardTitle>
      <span
        className={cn(
          "flex items-center gap-1 text-xs text-muted-foreground",
          overDownload && "font-medium text-destructive",
        )}
      >
        <span className="h-1.5 w-1.5 rounded-full" style={{ backgroundColor: colors.primary }} />
        Download {formatSpeedNow(downloadBps, unitNature)}
      </span>
      <span
        className={cn(
          "flex items-center gap-1 text-xs text-muted-foreground",
          overUpload && "font-medium text-destructive",
        )}
      >
        <span className="h-1.5 w-1.5 rounded-full" style={{ backgroundColor: colors.secondary }} />
        Upload {formatSpeedNow(uploadBps, unitNature)}
      </span>
      <div className="ml-auto flex flex-wrap items-center gap-1.5">
        <UnitSelect nature={unitNature} onChange={onUnitChange} />
        <WindowSelect windowMinutes={windowMinutes} onChange={onWindowChange} />
      </div>
    </CardHeader>
  );
}

// Grafico de linha de velocidade (download/upload) - ApexCharts (area
// com gradiente, curva suave, tooltip nativo). Janela e unidade
// controladas pelo pai (ver SPEED_CHART_WINDOWS/DEFAULT acima) pra
// ficarem em sincronia com o velocimetro ao lado
// (DeviceSpeedGaugeCard.tsx). Busca exatamente a janela selecionada
// (useDeviceSpeedHistory ja filtra no backend, sem over-fetch nem
// filtro no cliente).
export function DeviceSpeedChart({
  mac,
  windowMinutes,
  onWindowChange,
  unitNature,
  onUnitChange,
}: {
  mac: string;
  windowMinutes: number;
  onWindowChange: (minutes: number) => void;
  unitNature: ByteNature;
  onUnitChange: (nature: ByteNature) => void;
}) {
  const history = useDeviceSpeedHistory(mac, windowMinutes);
  const colors = useApexChartColors();
  const rawSamples = history.data ?? [];
  const samples = useMemo(() => smoothSpeedSamples(rawSamples), [rawSamples]);

  const maxAbs = Math.max(...samples.flatMap((sample) => [sample.downloadBps, sample.uploadBps]), 1);
  const { divisor, label } = pickByteScale(maxAbs, unitNature);

  const series = useMemo(
    () => [
      { name: "Download", data: samples.map((s) => ({ x: new Date(s.at).getTime(), y: s.downloadBps / divisor })) },
      { name: "Upload", data: samples.map((s) => ({ x: new Date(s.at).getTime(), y: s.uploadBps / divisor })) },
    ],
    [samples, divisor],
  );

  const options: ApexOptions = useMemo(
    () => ({
      chart: {
        type: "area",
        toolbar: { show: false },
        zoom: { enabled: false },
        animations: { enabled: false },
        fontFamily: "inherit",
        foreColor: colors.mutedForeground,
      },
      colors: [colors.primary, colors.secondary],
      stroke: { curve: "smooth", width: [2.5, 1.75] },
      fill: {
        type: "gradient",
        gradient: { shadeIntensity: 1, opacityFrom: 0.35, opacityTo: 0, stops: [0, 90, 100] },
      },
      dataLabels: { enabled: false },
      grid: { borderColor: colors.border, strokeDashArray: 3, padding: { left: 8, right: 8 } },
      xaxis: {
        type: "datetime",
        labels: { datetimeUTC: false, style: { colors: colors.mutedForeground, fontSize: "10px" } },
        axisBorder: { show: false },
        axisTicks: { show: false },
      },
      yaxis: {
        labels: {
          formatter: (value) => `${value.toFixed(value >= 10 ? 0 : 1)}${label}`,
          style: { colors: colors.mutedForeground, fontSize: "10px" },
        },
      },
      tooltip: {
        theme: "dark",
        x: { format: "HH:mm:ss" },
        y: { formatter: (value) => `${value.toFixed(2)}${label}/s` },
      },
      legend: { show: false },
    }),
    [colors, label],
  );

  return (
    <Card className="flex h-full flex-col">
      <SpeedChartHeader
        mac={mac}
        windowMinutes={windowMinutes}
        onWindowChange={onWindowChange}
        unitNature={unitNature}
        onUnitChange={onUnitChange}
      />
      <CardContent className="flex flex-1 flex-col">
        {!history.data ? (
          <div className="h-40 flex-1 animate-pulse rounded-lg bg-muted/30" />
        ) : samples.length < 2 ? (
          <p className="text-sm text-muted-foreground">Ainda sem amostras suficientes nessa janela.</p>
        ) : (
          <div className="flex flex-1 flex-col">
            <p className="mb-1 text-right text-[10px] text-muted-foreground">eixo em {label}/s</p>
            <div className="min-h-[220px] flex-1">
              <Chart options={options} series={series} type="area" height="100%" />
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
