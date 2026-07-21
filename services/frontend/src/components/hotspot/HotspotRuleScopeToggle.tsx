import { ArrowLeftRight, Users, type LucideIcon } from "lucide-react";
import { cn } from "@/lib/utils";

export type RuleScope = "within-profile" | "endpoints";

const SCOPES: { value: RuleScope; label: string; description: string; icon: LucideIcon }[] = [
  {
    value: "within-profile",
    label: "Dentro de um perfil",
    description: "Entre os clientes do mesmo perfil",
    icon: Users,
  },
  {
    value: "endpoints",
    label: "Entre origem e destino",
    description: "De um perfil/cliente para outro",
    icon: ArrowLeftRight,
  },
];

interface HotspotRuleScopeToggleProps {
  value: RuleScope;
  onChange: (value: RuleScope) => void;
}

// Seletor grande do escopo da regra (dentro de um perfil / entre origem
// e destino) - um botão-cartão por opção, ícone + descrição curta,
// mesmo padrão visual de HotspotLimitTypeToggle.
export function HotspotRuleScopeToggle({ value, onChange }: HotspotRuleScopeToggleProps) {
  return (
    <div className="grid gap-3 sm:grid-cols-2" role="radiogroup" aria-label="O que esta regra controla">
      {SCOPES.map((scope) => {
        const Icon = scope.icon;
        const active = value === scope.value;
        return (
          <button
            key={scope.value}
            type="button"
            role="radio"
            aria-checked={active}
            onClick={() => onChange(scope.value)}
            className={cn(
              "flex flex-col items-center gap-2 rounded-lg border p-4 text-center transition-colors",
              active
                ? "border-primary bg-primary/5 ring-1 ring-primary"
                : "border-border hover:border-primary/50 hover:bg-muted/40",
            )}
          >
            <Icon className={cn("h-6 w-6", active ? "text-primary" : "text-muted-foreground")} />
            <span className={cn("text-sm font-medium", active && "text-primary")}>{scope.label}</span>
            <span className="text-xs text-muted-foreground">{scope.description}</span>
          </button>
        );
      })}
    </div>
  );
}
