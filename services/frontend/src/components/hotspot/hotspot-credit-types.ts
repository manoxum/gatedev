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

export type HotspotCreditEntryType =
  | "manual_recharge"
  | "auto_recharge"
  | "voucher_redemption"
  | "session_active"
  | "session_closed";

export const HOTSPOT_CREDIT_ENTRY_KINDS = ["credit", "debit"] as const;
export type HotspotCreditEntryKind = (typeof HOTSPOT_CREDIT_ENTRY_KINDS)[number];

export function creditEntryKind(entryType: HotspotCreditEntryType): HotspotCreditEntryKind {
  return entryType === "session_active" || entryType === "session_closed" ? "debit" : "credit";
}

// balanceAfterBytes so vem preenchido para recarga/voucher - uma linha
// session_active/session_closed e o total agregado de uma sessao
// inteira (ver hotspot_sessions.go), o saldo mudou varias vezes dentro
// dela e o detalhe bruto de cada mudanca nao mora mais no Postgres.
// sessionId/startedAt/endedAt so vem preenchidos nas linhas de sessao -
// clicar nelas abre o detalhe de consumo daquela sessao (busca no
// Mongo).
export interface HotspotCreditHistoryEntry {
  entryType: HotspotCreditEntryType;
  amountBytes: number;
  balanceAfterBytes?: number;
  createdAt: string;
  sessionId?: number;
  startedAt?: string;
  endedAt?: string;
}
