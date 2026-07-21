import { Globe, ServerCog, Users, type LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";
import type { CommZone } from "@/components/hotspot/hotspot-isolation-types";

const ZONES: { value: CommZone; label: string; description: string; icon: LucideIcon }[] = [
  { value: "clients", label: "Entre clientes", description: "Um cliente falando com outro", icon: Users },
  { value: "wan", label: "Para a internet", description: "Saída dos clientes para fora", icon: Globe },
  { value: "local", label: "Para o painel/gateway", description: "Acesso aos serviços do host", icon: ServerCog },
];

interface HotspotFirewallZoneToggleProps {
  value: CommZone;
  onChange: (value: CommZone) => void;
}

// Seletor grande da zona da regra (entre clientes / internet / gateway)
// - mesmo padrão visual de HotspotLimitTypeToggle/HotspotRuleScopeToggle.
export function HotspotFirewallZoneToggle({ value, onChange }: HotspotFirewallZoneToggleProps) {
  return (
    <div className="grid gap-3 sm:grid-cols-3" role="radiogroup" aria-label="Zona da regra">
      {ZONES.map((zone) => {
        const Icon = zone.icon;
        const active = value === zone.value;
        return (
          <button
            key={zone.value}
            type="button"
            role="radio"
            aria-checked={active}
            onClick={() => onChange(zone.value)}
            className={cn(
              "flex flex-col items-center gap-2 rounded-lg border p-4 text-center transition-colors",
              active ? "border-primary bg-primary/5 ring-1 ring-primary" : "border-border hover:border-primary/50 hover:bg-muted/40",
            )}
          >
            <Icon className={cn("h-6 w-6", active ? "text-primary" : "text-muted-foreground")} />
            <span className={cn("text-sm font-medium", active && "text-primary")}>{zone.label}</span>
            <span className="text-xs text-muted-foreground">{zone.description}</span>
          </button>
        );
      })}
    </div>
  );
}
