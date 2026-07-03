import { cn } from "@/lib/utils";

export interface SpeedGaugeProps {
  /** Valor atual, em bits/segundo. */
  valueBps: number;
  /** Teto da escala (bits/segundo) - normalmente o limite configurado do dispositivo/global; se ausente, autoescala. */
  maxBps?: number | null;
  label: string;
  size?: "sm" | "lg";
  className?: string;
}

const SIZES = {
  sm: { width: 88, height: 54, stroke: 8, valueFont: 14, unitFont: 8, labelFont: 9 },
  lg: { width: 168, height: 100, stroke: 12, valueFont: 24, unitFont: 12, labelFont: 12 },
} as const;

const UNIT_STEP = 1000;

// pickUnit escolhe a unidade (bps/Kbps/Mbps/Gbps) pelo tamanho do
// valor - a mesma logica de "auto-unit" de qualquer medidor de rede
// (iftop, speedtest, etc.), sempre em base 1000 (bits, nao bytes).
function pickUnit(bps: number): { value: number; unit: string } {
  if (bps >= UNIT_STEP ** 3) return { value: bps / UNIT_STEP ** 3, unit: "Gbps" };
  if (bps >= UNIT_STEP ** 2) return { value: bps / UNIT_STEP ** 2, unit: "Mbps" };
  if (bps >= UNIT_STEP) return { value: bps / UNIT_STEP, unit: "Kbps" };
  return { value: bps, unit: "bps" };
}

function formatValue(value: number) {
  return value >= 10 ? value.toFixed(0) : value.toFixed(1);
}

// autoScaleMax evita um velocimetro "sempre no talo" quando nao ha
// limite configurado: usa um teto nominal (100 Mbps) ou 25% acima do
// valor atual, o que for maior, arredondado pra uma dezena redonda
// (em Mbps, depois convertido de volta pra bps).
function autoScaleMax(valueBps: number) {
  const nominalMbps = 100;
  const valueMbps = valueBps / UNIT_STEP ** 2;
  const withHeadroomMbps = Math.ceil((valueMbps * 1.25) / 10) * 10;
  return Math.max(nominalMbps, withHeadroomMbps) * UNIT_STEP ** 2;
}

function polarToCartesian(cx: number, cy: number, r: number, angleDeg: number) {
  const angleRad = (angleDeg * Math.PI) / 180;
  return { x: cx + r * Math.cos(angleRad), y: cy + r * Math.sin(angleRad) };
}

// Arco de 180 graus (semicirculo superior), de 180deg (esquerda) a 0deg
// (direita) - percent em [0,1] decide o quanto do arco fica preenchido.
function arcPath(cx: number, cy: number, r: number, percent: number) {
  const start = polarToCartesian(cx, cy, r, 180);
  const endAngle = 180 - 180 * Math.max(0, Math.min(1, percent));
  const end = polarToCartesian(cx, cy, r, endAngle);
  const largeArc = 180 * percent > 180 ? 1 : 0;
  return `M ${start.x} ${start.y} A ${r} ${r} 0 ${largeArc} 1 ${end.x} ${end.y}`;
}

// Velocimetro (meter semicircular): trilho na mesma cor da faixa em
// tom claro, arco preenchido na cor de destaque (ou destrutiva quando
// estoura o teto), numero central grande com unidade que se adapta a
// grandeza do valor (bps/Kbps/Mbps/Gbps) - sem marcações numéricas ao
// redor do arco (rótulo único, no centro).
export function SpeedGauge({ valueBps, maxBps, label, size = "lg", className }: SpeedGaugeProps) {
  const dims = SIZES[size];
  const max = maxBps && maxBps > 0 ? maxBps : autoScaleMax(valueBps);
  const percent = Math.max(0, Math.min(1, valueBps / max));
  const overLimit = !!maxBps && valueBps > maxBps;
  const { value: displayValue, unit } = pickUnit(valueBps);

  const cx = dims.width / 2;
  const cy = dims.height - dims.stroke;
  const r = cy - dims.stroke / 2;
  const trackPath = arcPath(cx, cy, r, 1);
  const valuePath = arcPath(cx, cy, r, percent);

  return (
    <div className={cn("flex flex-col items-center", className)}>
      <svg width={dims.width} height={dims.height} viewBox={`0 0 ${dims.width} ${dims.height}`}>
        <path
          d={trackPath}
          fill="none"
          stroke="hsl(var(--primary) / 0.15)"
          strokeWidth={dims.stroke}
          strokeLinecap="round"
        />
        <path
          d={valuePath}
          fill="none"
          stroke={overLimit ? "hsl(var(--destructive))" : "hsl(var(--primary))"}
          strokeWidth={dims.stroke}
          strokeLinecap="round"
        />
        <text
          x={cx}
          y={cy - dims.stroke - dims.unitFont * 0.9}
          textAnchor="middle"
          className="fill-foreground font-semibold"
          style={{ fontSize: dims.valueFont }}
        >
          {formatValue(displayValue)}
        </text>
        <text
          x={cx}
          y={cy - dims.stroke + dims.unitFont * 0.6}
          textAnchor="middle"
          className="fill-muted-foreground"
          style={{ fontSize: dims.unitFont }}
        >
          {unit}
        </text>
      </svg>
      <p className="-mt-1 text-center text-muted-foreground" style={{ fontSize: dims.labelFont }}>
        {label}
      </p>
    </div>
  );
}
