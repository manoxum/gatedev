import { RATE_UNIT_LABELS } from "@/components/hotspot/hotspot-limits-types";

// Opções do seletor de unidade de taxa/tamanho: bits (Kb/Mb/Gb) e
// bytes (KB/MB/GB) - ver RateUnit em hotspot-limits-types.ts. Usado
// tanto nos campos de Taxa/Cota de dispositivo/perfil quanto na
// quantidade da recarga e no extrato de crédito (device-detail/).
export function RateUnitOptions() {
  return (
    <>
      {Object.entries(RATE_UNIT_LABELS).map(([value, label]) => (
        <option key={value} value={value}>
          {label}
        </option>
      ))}
    </>
  );
}
