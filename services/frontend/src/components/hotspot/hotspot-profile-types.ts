import type { HotspotLimits, QuotaPeriod } from "@/components/hotspot/hotspot-limits-types";

// Perfil de dispositivo - bundle nomeado e reutilizavel de limites
// (mesmo shape de HotspotLimits) + politica de credito, que um
// dispositivo herda por padrao (perfil "Padrão", isDefault=true).
// Override explicito por dispositivo (aba Limites/Crédito) sempre
// vence sobre o perfil vinculado.
export interface HotspotProfile extends HotspotLimits {
  id: string;
  name: string;
  isDefault: boolean;
  creditRechargeAmountBytes: number | null;
  creditRechargePeriod: QuotaPeriod | null;
  creditPlafondBytes: number | null;
}

export type HotspotProfileRequest = Omit<HotspotProfile, "id" | "isDefault">;
