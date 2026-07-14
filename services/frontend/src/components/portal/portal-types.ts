import type { HotspotDeviceQuotaPeriodUsage, LimitType } from "@/components/hotspot/hotspot-limits-types";

// Resposta de GET /api/hotspot/portal/me - reusa os mesmos nomes de
// campo do backend (services/backend/hotspot_portal.go).
export interface PortalMeResponse {
  mac: string;
  alias?: string;
  profileName?: string;
  blocked: boolean;
  limitType: LimitType;
  blockedByCredit: boolean;
  balanceBytes: number;
  plafondBytes: number | null;
  quotaPeriods?: HotspotDeviceQuotaPeriodUsage[];
}

export interface PortalCreditHistoryEntry {
  entryType: string;
  amountBytes: number;
  balanceAfterBytes: number;
  createdAt: string;
}
