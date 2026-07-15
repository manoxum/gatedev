import { SignalHigh, SignalLow, SignalMedium, SignalZero } from "lucide-react";
import { cn } from "@/lib/utils";

// Faixas de qualidade de sinal Wi-Fi em dBm (RSSI) - convenção comum de
// mercado (roteadores/apps de site survey costumam usar cortes bem
// próximos destes). Quanto MAIS PRÓXIMO de 0 (menos negativo), melhor -
// ex.: -45 dBm é ótimo, -80 dBm é péssimo.
const SIGNAL_TIERS = [
  { min: -55, label: "Excelente", icon: SignalHigh, colorClass: "text-primary" },
  { min: -67, label: "Bom", icon: SignalMedium, colorClass: "text-primary" },
  { min: -75, label: "Regular", icon: SignalLow, colorClass: "text-amber-500 dark:text-amber-400" },
  { min: -Infinity, label: "Fraco", icon: SignalZero, colorClass: "text-destructive" },
] as const;

export function signalTier(dbm: number | null | undefined) {
  if (dbm == null) return null;
  return SIGNAL_TIERS.find((tier) => dbm >= tier.min) ?? SIGNAL_TIERS[SIGNAL_TIERS.length - 1];
}

interface WifiSignalIndicatorProps {
  dbm?: number | null;
  /** "sm": icone + dBm em linha (tabela). "lg": icone maior + rotulo de qualidade (detalhe). */
  size?: "sm" | "lg";
  className?: string;
}

// Indicador de intensidade de sinal Wi-Fi (RSSI em dBm) reaproveitado
// pela listagem de clientes (HotspotClientsCard.tsx) e pela visão geral
// do dispositivo (DeviceOverviewTab.tsx) - mesma fonte de verdade
// (signalTier) garante que o ícone/cor nunca diverge entre as duas
// telas. Sem leitura (dispositivo offline, ou driver sem suporte a
// "iw station dump" - ver hotspotClientSignal no worker) cai no
// SignalZero neutro em vez de esconder o indicador, já que o campo é
// omitido pelo backend nesse caso (signalDbm undefined).
export function WifiSignalIndicator({ dbm, size = "sm", className }: WifiSignalIndicatorProps) {
  const tier = signalTier(dbm);
  const Icon = tier?.icon ?? SignalZero;
  const colorClass = tier?.colorClass ?? "text-muted-foreground/50";

  if (size === "lg") {
    return (
      <div className={cn("flex items-center gap-3 rounded-lg border border-border/60 bg-muted/30 px-3 py-2.5", className)}>
        <div className={cn("flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-primary/10", colorClass)}>
          <Icon className="h-4 w-4" />
        </div>
        <div className="min-w-0">
          <p className="text-xs text-muted-foreground">Sinal Wi-Fi</p>
          <p className="truncate text-sm font-semibold">
            {tier ? tier.label : "desconhecido"}
            {dbm != null && <span className="ml-1.5 font-normal text-muted-foreground">{dbm} dBm</span>}
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className={cn("flex items-center gap-1.5", className)} title={tier ? `${tier.label} (${dbm} dBm)` : "sinal desconhecido"}>
      <Icon className={cn("h-4 w-4 shrink-0", colorClass)} />
      <span className="text-xs text-muted-foreground">{dbm != null ? `${dbm} dBm` : "—"}</span>
    </div>
  );
}
