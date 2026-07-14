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

// HotspotGlobalLimits e o limite global (singleton, sempre ativo,
// nunca fallback de perfil/dispositivo) - shape antigo, com cota unica
// + throttle, preservado so aqui. Ver services/backend/hotspot_global_limits.go.
export interface HotspotGlobalLimits {
  downloadRateValue: number | null;
  downloadRateUnit: RateUnit;
  uploadRateValue: number | null;
  uploadRateUnit: RateUnit;
  quotaBytes: number | null;
  quotaUnit: RateUnit;
  quotaPeriod: QuotaPeriod | null;
  quotaThrottleDownloadValue: number | null;
  quotaThrottleDownloadUnit: RateUnit;
  quotaThrottleUploadValue: number | null;
  quotaThrottleUploadUnit: RateUnit;
}

// Tipo unico e mutuamente exclusivo de limitacao de um dispositivo
// (override) ou perfil - substitui a combinacao livre de cota+credito
// que causava o bug de "cota para de contabilizar" (um perfil com os
// dois habilitados ao mesmo tempo ativava debito de credito por baixo,
// que bloqueava o dispositivo de verdade). Ver services/backend/hotspot_device_limits.go.
//
// "custom" so e valido em PERFIL: o perfil nao aplica limite nenhum -
// o dispositivo vinculado a ele e quem escolhe a propria estrategia
// (nunca "custom" ele mesmo). Componentes que renderizam o seletor pro
// dispositivo devem omitir essa opcao (ver includeCustom em
// HotspotLimitTypeToggle.tsx).
export type LimitType = "unlimited" | "credit" | "quota" | "custom";

export const LIMIT_TYPE_LABELS: Record<LimitType, string> = {
  unlimited: "Ilimitado",
  credit: "Crédito",
  quota: "Cota",
  custom: "Customizado",
};

// HotspotLimits e o shape de limite de um dispositivo (override) ou
// perfil - LimitType decide qual bloco esta em uso: nenhum (unlimited),
// a politica de credito (ver hotspot-credit-types.ts), ou ate os 3
// tetos de cota abaixo em simultaneo (cada um com seu proprio
// acumulador, ver HotspotDeviceQuotaPeriodUsage). Taxa continua
// independente do tipo, sempre configuravel. Nao confundir com
// HotspotGlobalLimits acima - o limite global fica fora deste
// redesenho.
export interface HotspotLimits {
  downloadRateValue: number | null;
  downloadRateUnit: RateUnit;
  uploadRateValue: number | null;
  uploadRateUnit: RateUnit;
  limitType: LimitType;
  dailyQuotaBytes: number | null;
  dailyQuotaUnit: RateUnit;
  weeklyQuotaBytes: number | null;
  weeklyQuotaUnit: RateUnit;
  monthlyQuotaBytes: number | null;
  monthlyQuotaUnit: RateUnit;
}

// Resposta de GET /hotspot/devices/{mac}/limits - sempre os limites
// EFETIVOS (herdados do perfil, ou o override proprio do dispositivo
// quando o perfil vinculado e "custom"), acompanhados do nome/tipo do
// perfil pra decidir se mostra um resumo so-leitura ("herdado do
// perfil X") ou o formulario editavel (so quando profileLimitType ===
// "custom" - ver DeviceLimitsTab.tsx).
export interface HotspotDeviceLimitsResponse extends HotspotLimits {
  profileName: string;
  profileLimitType: LimitType;
}

// Acumulado do periodo corrente de UM dos 3 tetos possiveis de UM
// dispositivo - so existe entrada para o periodo que foi efetivamente
// configurado (GET /hotspot/devices/{mac}/quota so devolve os
// configurados). "blocked" e bloqueio rigido (nunca throttle) - ver
// services/backend/hotspot_device_quota_store.go.
export interface HotspotDeviceQuotaPeriodUsage {
  periodType: QuotaPeriod;
  quotaBytes: number;
  quotaUnit: RateUnit;
  downloadBytes: number;
  uploadBytes: number;
  periodStart: string;
  periodEnd: string;
  blocked: boolean;
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

// O backend devolve downloadBps/uploadBps (ver HotspotDeviceStats em
// useHotspotQueries.ts) em BYTES por segundo (nome historico da API);
// os velocimetros trabalham em BITS por segundo (convenção de rede -
// Mbps/Gbps sempre são bits, nunca bytes).
export function toBitsPerSecond(bytesPerSecond: number) {
  return bytesPerSecond * 8;
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

// Formata um total de dados na unidade original em que foi informado
// (ex.: valor de voucher emitido em MB continua exibido em MB, em vez
// de sempre converter para GB) - ver hotspot-voucher-*.
export function formatQuotaValue(bytes: number, unit: RateUnit): string {
  return `${bytesToQuotaValue(bytes, unit).toFixed(0)}${RATE_UNIT_LABELS[unit]}`;
}

// Natureza da grandeza (bit ou byte) - diferente de RateUnit, que
// tambem embute a escala (kilo/mega/giga). Usado onde a escala precisa
// se adaptar ao tamanho de cada valor individualmente (ver
// autoScaleBytes) e só a natureza bit/byte é uma escolha do operador.
export type ByteNature = "byte" | "bit";

const AUTO_SCALE_STEP = 1000;
const BYTE_UNIT_LABELS = ["B", "KB", "MB", "GB", "TB"];
const BIT_UNIT_LABELS = ["b", "Kb", "Mb", "Gb", "Tb"];

// autoScaleBytes escolhe a escala (B/KB/MB/GB/TB, ou a versão em bits)
// pelo tamanho de CADA valor - mesma lógica de "auto-unit" do
// SpeedGauge (components/ui/speed-gauge.tsx), sempre base 1000. Existe
// porque uma unidade fixa (RateUnit) faz consumo pequeno aparecer como
// "0.00GB" ao lado de recargas grandes na mesma lista (ver
// DeviceMovementsCard.tsx) - cada linha deve mostrar a grandeza que
// faz sentido pra ela, não uma unidade global pra tabela inteira.
export function autoScaleBytes(bytes: number, nature: ByteNature = "byte"): { value: number; label: string } {
  const labels = nature === "bit" ? BIT_UNIT_LABELS : BYTE_UNIT_LABELS;
  const raw = nature === "bit" ? bytes * 8 : bytes;
  const sign = raw < 0 ? -1 : 1;
  let magnitude = Math.abs(raw);
  let index = 0;
  while (magnitude >= AUTO_SCALE_STEP && index < labels.length - 1) {
    magnitude /= AUTO_SCALE_STEP;
    index += 1;
  }
  return { value: sign * magnitude, label: labels[index] };
}

export function formatAutoScaleBytes(bytes: number, nature: ByteNature = "byte"): string {
  const { value, label } = autoScaleBytes(bytes, nature);
  return `${value.toFixed(2)}${label}`;
}

// pickByteScale escolhe UMA escala pra converter uma serie inteira de
// valores (nunca uma por valor, como autoScaleBytes) - um grafico so
// pode ter um eixo, entao todo ponto tem que dividir pelo mesmo
// divisor. Passe o maior valor absoluto da serie; use o divisor
// devolvido pra converter cada ponto (bytes / divisor).
export function pickByteScale(maxAbsBytes: number, nature: ByteNature = "byte"): { divisor: number; label: string } {
  const labels = nature === "bit" ? BIT_UNIT_LABELS : BYTE_UNIT_LABELS;
  const bitFactor = nature === "bit" ? 8 : 1;
  let scaled = maxAbsBytes * bitFactor;
  // divisor precisa satisfazer "valorOriginalEmBytes / divisor ==
  // scaled" no final do loop - como scaled comeca multiplicado por
  // bitFactor (bytes -> bits) e so DEPOIS dividido por 1000 a cada
  // passo, divisor tem que comecar no INVERSO de bitFactor (nao no
  // proprio bitFactor) pra essa igualdade se manter passo a passo.
  // Comecar em "bitFactor" (bug antigo) deixava divisor 64x maior que o
  // certo pra natureza "bit" (bitFactor=8, erro de bitFactor^2) -
  // qualquer serie exibida em bits/s aparecia com o valor errado.
  let divisor = 1 / bitFactor;
  let index = 0;
  while (scaled >= AUTO_SCALE_STEP && index < labels.length - 1) {
    scaled /= AUTO_SCALE_STEP;
    divisor *= AUTO_SCALE_STEP;
    index += 1;
  }
  return { divisor, label: labels[index] };
}

// formatSpeedNow formata bytes/s num texto curto ("12.3Mbps"/"850KB/s")
// respeitando a natureza bits/bytes - mesma logica de arredondamento do
// SpeedGauge (1 casa decimal abaixo de 10, inteiro dali pra cima), so
// que como texto solto em vez de arco SVG. Usada tanto no cabecalho de
// DeviceSpeedChart.tsx quanto nas linhas compactas de
// HotspotClientsCard.tsx.
export function formatSpeedNow(bytesPerSecond: number, nature: ByteNature) {
  const { divisor, label } = pickByteScale(bytesPerSecond, nature);
  const value = bytesPerSecond / divisor;
  return `${value >= 10 ? value.toFixed(0) : value.toFixed(1)}${label}/s`;
}
