import { CreditCard, Gauge, Infinity as InfinityIcon, SlidersHorizontal, type LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";
import { LIMIT_TYPE_LABELS, type LimitType } from "@/components/hotspot/hotspot-limits-types";

const LIMIT_TYPE_ICONS: Record<LimitType, LucideIcon> = {
  unlimited: InfinityIcon,
  credit: CreditCard,
  quota: Gauge,
  custom: SlidersHorizontal,
};

const LIMIT_TYPE_DESCRIPTIONS: Record<LimitType, string> = {
  unlimited: "Sem teto de cota nem crédito",
  credit: "Precisa de saldo para navegar",
  quota: "Teto diário/semanal/mensal",
  custom: "Não aplica limite - o dispositivo define",
};

const DEVICE_LIMIT_TYPES: LimitType[] = ["unlimited", "credit", "quota"];
const PROFILE_LIMIT_TYPES: LimitType[] = ["unlimited", "credit", "quota", "custom"];

interface HotspotLimitTypeToggleProps {
  value: LimitType;
  onChange: (value: LimitType) => void;
  // "custom" so faz sentido em perfil (delega a decisao pro
  // dispositivo) - default false pra nunca aparecer como opcao no
  // formulario de dispositivo, que e sempre o ultimo nivel.
  includeCustom?: boolean;
}

// Seletor grande do tipo de limitação (ilimitado/crédito/cota[/customizado])
// - um botão-cartão por tipo, com ícone + descrição curta, no lugar de
// um <select> nativo. Reusado por HotspotDeviceLimitsForm e
// HotspotProfileForm via HotspotLimitTypeFields.tsx.
export function HotspotLimitTypeToggle({ value, onChange, includeCustom = false }: HotspotLimitTypeToggleProps) {
  const types = includeCustom ? PROFILE_LIMIT_TYPES : DEVICE_LIMIT_TYPES;
  return (
    <div
      className={cn("grid gap-3", types.length === 4 ? "sm:grid-cols-2 lg:grid-cols-4" : "sm:grid-cols-3")}
      role="radiogroup"
      aria-label="Tipo de limitação"
    >
      {types.map((type) => {
        const Icon = LIMIT_TYPE_ICONS[type];
        const active = value === type;
        return (
          <button
            key={type}
            type="button"
            role="radio"
            aria-checked={active}
            onClick={() => onChange(type)}
            className={cn(
              "flex flex-col items-center gap-2 rounded-lg border p-4 text-center transition-colors",
              active
                ? "border-primary bg-primary/5 ring-1 ring-primary"
                : "border-border hover:border-primary/50 hover:bg-muted/40",
            )}
          >
            <Icon className={cn("h-6 w-6", active ? "text-primary" : "text-muted-foreground")} />
            <span className={cn("text-sm font-medium", active && "text-primary")}>{LIMIT_TYPE_LABELS[type]}</span>
            <span className="text-xs text-muted-foreground">{LIMIT_TYPE_DESCRIPTIONS[type]}</span>
          </button>
        );
      })}
    </div>
  );
}
