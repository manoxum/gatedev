export type QuotaPeriod = "daily" | "weekly" | "monthly";

// Unidade de taxa: bits/s (kbit/mbit/gbit) ou bytes/s (kbyte/mbyte/
// gbyte) - o worker traduz kbyte/mbyte/gbyte para os sufixos tc
// kbps/mbps/gbps (nome confuso do tc, mas e assim que ele distingue
// bytes/s de bits/s). Ver services/worker/controller/shaping_tc.go.
export type RateUnit = "kbit" | "mbit" | "gbit" | "kbyte" | "mbyte" | "gbyte";

// Rotulo curto de cada unidade, usado tanto nas <option> do seletor
// (RateUnitOptions.tsx) quanto no sufixo exibido junto a valores
// convertidos (ex.: extrato de credito).
export const RATE_UNIT_LABELS: Record<RateUnit, string> = {
  kbit: "Kb",
  mbit: "Mb",
  gbit: "Gb",
  kbyte: "KB",
  mbyte: "MB",
  gbyte: "GB",
};

export interface HotspotLimits {
  downloadRateValue: number | null;
  downloadRateUnit: RateUnit;
  uploadRateValue: number | null;
  uploadRateUnit: RateUnit;
  quotaBytes: number | null;
  quotaPeriod: QuotaPeriod | null;
  quotaThrottleDownloadValue: number | null;
  quotaThrottleDownloadUnit: RateUnit;
  quotaThrottleUploadValue: number | null;
  quotaThrottleUploadUnit: RateUnit;
}

export interface HotspotTraffic {
  downloadBytes: number;
  uploadBytes: number;
  periodStart: string;
  periodEnd: string;
  throttled: boolean;
  quotaBytes: number | null;
  quotaPeriod: QuotaPeriod | null;
}

export const GIGABYTE = 1024 * 1024 * 1024;

export function bytesToGB(bytes: number) {
  return bytes / GIGABYTE;
}

// Bits por segundo de cada unidade, usado para converter a taxa
// configurada (valor + unidade) para bps antes de alimentar o
// velocimetro (SpeedGauge trabalha sempre em bits/s).
const RATE_UNIT_BITS_PER_SECOND: Record<RateUnit, number> = {
  kbit: 1_000,
  mbit: 1_000_000,
  gbit: 1_000_000_000,
  kbyte: 8_000,
  mbyte: 8_000_000,
  gbyte: 8_000_000_000,
};

export function rateToBps(value: number | null, unit: RateUnit): number | null {
  if (!value) return null;
  return value * RATE_UNIT_BITS_PER_SECOND[unit];
}

// Mesma tabela de unidades usada na Taxa, mas aqui sem a dimensão "por
// segundo" - só a escala (kilo/mega/giga, bit ou byte) para converter
// um total de dados (cota) de/para bytes.
function unitToBytesFactor(unit: RateUnit): number {
  return RATE_UNIT_BITS_PER_SECOND[unit] / 8;
}

export function quotaValueToBytes(value: number, unit: RateUnit): number {
  return Math.round(value * unitToBytesFactor(unit));
}

export function bytesToQuotaValue(bytes: number, unit: RateUnit): number {
  return bytes / unitToBytesFactor(unit);
}
