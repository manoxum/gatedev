import { useState } from "react";
import { pickByteScale, type ByteNature } from "@/components/hotspot/hotspot-limits-types";
import type { HotspotSessionConsumptionEntry } from "@/components/hotspot/hotspot-session-types";

interface SessionConsumptionChartProps {
  entries: HotspotSessionConsumptionEntry[];
  nature: ByteNature;
}

const WIDTH = 600;
const HEIGHT = 140;
const PADDING_X = 8;
const PADDING_TOP = 8;
const BASELINE_Y = HEIGHT - 20;
const MAX_BAR_WIDTH = 24;
const GAP = 2;
const RADIUS = 4;

// Caminho com cantos arredondados so em cima (base quadrada), regra da
// skill de dataviz pra barras - um <rect rx=...> arredondaria os 4
// cantos.
function topRoundedBarPath(x: number, y: number, width: number, height: number) {
  const r = Math.max(0, Math.min(RADIUS, width / 2, height));
  if (height <= 0) return "";
  return `M ${x} ${y + height} L ${x} ${y + r} Q ${x} ${y} ${x + r} ${y} L ${x + width - r} ${y} Q ${x + width} ${y} ${x + width} ${y + r} L ${x + width} ${y + height} Z`;
}

// Grafico de barras do consumo bruto de uma sessao (mesmos dados da
// tabela ao lado, ver DeviceMovementsCard.tsx) - serie unica, sem
// legenda (o titulo ja diz o que e), com tooltip por barra ao passar o
// mouse/focar. Ordem cronologica (mais antigo primeiro, oposto da
// tabela que mostra mais recente primeiro) pra ler como uma linha do
// tempo da esquerda pra direita. Todas as barras dividem pela MESMA
// escala (pickByteScale, a partir do maior valor da serie) - um
// grafico so pode ter um eixo, diferente da tabela onde cada linha
// escolhe a propria grandeza.
export function SessionConsumptionChart({ entries, nature }: SessionConsumptionChartProps) {
  const [hoverIndex, setHoverIndex] = useState<number | null>(null);
  if (entries.length === 0) return null;

  const chronological = [...entries].reverse();
  const maxAbs = Math.max(...chronological.map((entry) => Math.abs(entry.amountBytes)), 1);
  const { divisor, label } = pickByteScale(maxAbs, nature);

  const plotWidth = WIDTH - PADDING_X * 2;
  const slot = plotWidth / chronological.length;
  const barWidth = Math.max(2, Math.min(MAX_BAR_WIDTH, slot - GAP));
  const plotHeight = BASELINE_Y - PADDING_TOP;
  const hovered = hoverIndex !== null ? chronological[hoverIndex] : null;

  return (
    <div className="space-y-1">
      <div className="flex items-baseline justify-between">
        <p className="text-xs font-medium text-muted-foreground">Consumo por evento</p>
        <p className="text-xs text-muted-foreground">em {label}</p>
      </div>
      <div className="relative">
        <svg viewBox={`0 0 ${WIDTH} ${HEIGHT}`} className="h-36 w-full" preserveAspectRatio="none">
          <line
            x1={PADDING_X}
            y1={BASELINE_Y}
            x2={WIDTH - PADDING_X}
            y2={BASELINE_Y}
            stroke="hsl(var(--muted-foreground) / 0.3)"
            strokeWidth={1}
          />
          {chronological.map((entry, index) => {
            const magnitude = Math.abs(entry.amountBytes);
            const barHeight = (magnitude / maxAbs) * plotHeight;
            const x = PADDING_X + index * slot + (slot - barWidth) / 2;
            const y = BASELINE_Y - barHeight;
            return (
              <path
                key={`${entry.createdAt}-${index}`}
                d={topRoundedBarPath(x, y, barWidth, Math.max(barHeight, 1))}
                fill={hoverIndex === index ? "hsl(var(--primary))" : "hsl(var(--primary) / 0.75)"}
                tabIndex={0}
                onMouseEnter={() => setHoverIndex(index)}
                onMouseLeave={() => setHoverIndex((current) => (current === index ? null : current))}
                onFocus={() => setHoverIndex(index)}
                onBlur={() => setHoverIndex((current) => (current === index ? null : current))}
              >
                <title>
                  {new Date(entry.createdAt).toLocaleString()} — {(magnitude / divisor).toFixed(2)}
                  {label}
                </title>
              </path>
            );
          })}
        </svg>
        {hovered && (
          <div
            className="pointer-events-none absolute top-1 -translate-x-1/2 rounded-md border bg-popover px-2 py-1 text-xs shadow-md"
            style={{ left: `${((hoverIndex! + 0.5) / chronological.length) * 100}%` }}
          >
            <p className="font-semibold">
              {(Math.abs(hovered.amountBytes) / divisor).toFixed(2)}
              {label}
            </p>
            <p className="text-muted-foreground">{new Date(hovered.createdAt).toLocaleString()}</p>
          </div>
        )}
      </div>
    </div>
  );
}
