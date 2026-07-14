import GaugeComponent from "react-gauge-component";
import { cn } from "@/lib/utils";
import { pickByteScale, type ByteNature } from "@/components/hotspot/hotspot-limits-types";

export interface SpeedGaugeProps {
  /** Valor atual, em bits/segundo. */
  valueBps: number;
  /** Teto da escala (bits/segundo) - normalmente o limite configurado do dispositivo/global; se ausente, autoescala. */
  maxBps?: number | null;
  label: string;
  size?: "sm" | "lg";
  className?: string;
  /** Grandeza de exibicao do valor central (bits ou bytes) - "bit" por padrao. */
  unitNature?: ByteNature;
}

// Baseado em react-gauge-component (biblioteca dedicada a gauges KPI,
// nao mais SVG desenhado a mao) - o arco a mao vinha reincidindo em
// bugs de geometria a cada ajuste. Semicirculo sem ponteiro, "cheio ate
// o valor atual" via dois subArcs (cor viva ate o valor, trilho neutro
// dali ate o teto) - ver arc.subArcs abaixo.
const SIZES = {
  sm: { width: 116, height: 78, arcWidth: 0.32, valueFont: 16, unitFont: 9, labelFont: 9 },
  lg: { width: 176, height: 118, arcWidth: 0.3, valueFont: 24, unitFont: 12, labelFont: 12 },
} as const;

const UNIT_STEP = 1000;

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

// arcColorFor muda a cor gradualmente conforme o valor se aproxima do
// teto (verde -> ambar -> vermelho), nao so um salto binario pra
// vermelho ao estourar. So faz sentido quando ha um teto REAL
// configurado pelo admin (hasRealLimit) - no autoescala (sem limite
// nenhum) o "teto" e so uma referencia visual arbitraria (sempre ~80%
// cheio por construcao, ver autoScaleMax), "perto do limite" não
// significa nada ali, entao fica sempre na cor neutra.
function arcColorFor(percent: number, hasRealLimit: boolean) {
  if (!hasRealLimit) return "hsl(var(--primary))";
  if (percent >= 1) return "hsl(var(--destructive))";
  if (percent >= 0.85) return "hsl(24, 94%, 53%)"; // laranja
  if (percent >= 0.7) return "hsl(38, 92%, 50%)"; // ambar
  return "hsl(var(--primary))";
}

export function SpeedGauge({ valueBps, maxBps, label, size = "lg", className, unitNature = "bit" }: SpeedGaugeProps) {
  const dims = SIZES[size];
  const hasRealLimit = !!maxBps && maxBps > 0;
  const max = hasRealLimit ? maxBps! : autoScaleMax(valueBps);
  const value = Math.max(0, Math.min(valueBps, max));
  // pickByteScale trabalha em bytes/s (mesma convencao do resto do
  // painel - ver formatSpeedNow) - valueBps aqui e sempre bits/s
  // (contrato do componente), entao converte antes.
  const { divisor, label: unit } = pickByteScale(valueBps / 8, unitNature);
  const displayValue = valueBps / 8 / divisor;
  const percent = max > 0 ? value / max : 0;
  const arcColor = arcColorFor(percent, hasRealLimit);
  // "cheio" (0..value) na cor viva, "vazio" (value..max) no trilho
  // neutro - e o que da o efeito "progresso", ja que a lib por padrao
  // colore o arco inteiro em faixas fixas (uso tipico de velocimetro
  // com ponteiro), nao um preenchimento parcial.
  const subArcs = value > 0 ? [{ limit: value, color: arcColor }, { color: "hsl(var(--muted-foreground) / 0.15)" }] : [{ color: "hsl(var(--muted-foreground) / 0.15)" }];

  return (
    <div className={cn("flex flex-col items-center", className)}>
      <div style={{ width: dims.width, height: dims.height }}>
        <GaugeComponent
          type="semicircle"
          value={value}
          minValue={0}
          maxValue={max}
          arc={{ width: dims.arcWidth, padding: 0, cornerRadius: 4, subArcs }}
          pointer={{ hide: true }}
          labels={{
            valueLabel: {
              hide: true,
            },
            tickLabels: {
              hideMinMax: true,
              ticks: [],
            },
          }}
        />
      </div>
      <div className="-mt-3 flex items-baseline gap-1">
        <span className="font-bold text-foreground" style={{ fontSize: dims.valueFont, fontVariantNumeric: "tabular-nums" }}>
          {formatValue(displayValue)}
        </span>
        <span className="font-semibold text-muted-foreground" style={{ fontSize: dims.unitFont }}>
          {unit}/s
        </span>
      </div>
      <p className="text-center text-muted-foreground" style={{ fontSize: dims.labelFont }}>
        {label}
      </p>
    </div>
  );
}
