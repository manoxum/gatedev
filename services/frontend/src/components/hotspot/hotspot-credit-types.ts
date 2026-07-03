import type { QuotaPeriod } from "@/components/hotspot/hotspot-limits-types";

export interface HotspotCredit {
  enabled: boolean;
  balanceBytes: number;
  rechargeAmountBytes: number | null;
  rechargePeriod: QuotaPeriod | null;
  plafondBytes: number | null;
  nextRechargeAt: string | null;
  blockedByCredit: boolean;
}
