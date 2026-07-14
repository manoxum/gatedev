import { useMemo } from "react";
import Chart from "react-apexcharts";
import type { ApexOptions } from "apexcharts";
import { SpeedGauge } from "@/components/ui/speed-gauge";
import { useApexChartColors } from "@/components/hotspot/apex-chart-theme";
import { pickByteScale, toBitsPerSecond } from "@/components/hotspot/hotspot-limits-types";
import { useGlobalSpeedHistory, useGlobalStats } from "@/components/hotspot/useHotspotQueries";

const WINDOW_MINUTES = 5;

// Painel geral (todo o trafego do hotspot, nao um dispositivo) do card
// "Configuracao atual" - velocimetros "agora" e tendencia (5 min) num
// so painel com moldura unica (nao dois elementos soltos lado a lado),
// pra ler como um widget so. Grafico via ApexCharts (area com
// gradiente, curva suave, tooltip nativo) - mesmo padrao de
// device-detail/DeviceSpeedChart.tsx. Usa os mesmos hooks/rotas globais
// (useGlobalStats/useGlobalSpeedHistory) que alimentam o velocimetro
// "ao vivo" e o historico por segundo, so que agregados (mac vazio no
// worker) em vez de por dispositivo.
export function HotspotGlobalSpeedPanel() {
  const stats = useGlobalStats();
  const history = useGlobalSpeedHistory(WINDOW_MINUTES);
  const colors = useApexChartColors();

  const samples = history.data ?? [];
  const hasTrend = samples.length >= 2;
  const maxAbs = Math.max(...samples.flatMap((sample) => [sample.downloadBps, sample.uploadBps]), 1);
  const { divisor, label } = pickByteScale(maxAbs, "bit");

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
        sparkline: { enabled: false },
      },
      colors: [colors.primary, colors.secondary],
      stroke: { curve: "smooth", width: [2.5, 1.75] },
      fill: {
        type: "gradient",
        gradient: { shadeIntensity: 1, opacityFrom: 0.35, opacityTo: 0, stops: [0, 90, 100] },
      },
      dataLabels: { enabled: false },
      grid: { borderColor: colors.border, strokeDashArray: 3, padding: { left: 8, right: 8, top: -20 } },
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
    <div className="flex w-full flex-1 flex-col items-center gap-4 rounded-xl border border-border/60 bg-muted/20 p-4 sm:flex-row sm:items-center">
      <div className="flex shrink-0 items-center justify-center gap-4">
        <SpeedGauge valueBps={toBitsPerSecond(stats.data?.downloadBps ?? 0)} label="Download geral" size="lg" />
        <SpeedGauge valueBps={toBitsPerSecond(stats.data?.uploadBps ?? 0)} label="Upload geral" size="lg" />
      </div>
      <div className="hidden self-stretch border-l border-border/60 sm:block" />
      <div className="flex min-w-0 flex-1 flex-col gap-1.5">
        <p className="text-[11px] font-medium text-muted-foreground">Tendência geral · últimos 5 min</p>
        {hasTrend ? (
          <Chart options={options} series={series} type="area" height={140} />
        ) : (
          <div className="flex h-[140px] w-full items-center justify-center text-xs text-muted-foreground">
            sem amostras ainda
          </div>
        )}
      </div>
    </div>
  );
}
