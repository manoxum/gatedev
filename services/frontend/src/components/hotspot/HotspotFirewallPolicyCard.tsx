import { Globe, ServerCog } from "lucide-react";
import { SelectNative } from "@/components/ui/select-native";
import {
  useHotspotFirewallPolicy,
  useHotspotFirewallPolicyMutation,
  type FirewallPolicy,
} from "@/components/hotspot/useHotspotFirewallPolicy";

// Política padrão das zonas wan (internet) e local (painel/gateway):
// o que acontece com o tráfego que nenhuma regra cobre. Default
// "permitir" - só o operador que quiser um firewall restritivo muda
// para "bloquear" (aí só o que as regras liberarem passa; DNS/DHCP/
// painel continuam sempre acessíveis por segurança).
export function HotspotFirewallPolicyCard() {
  const policy = useHotspotFirewallPolicy();
  const mutation = useHotspotFirewallPolicyMutation();
  const current: FirewallPolicy = policy.data ?? { wanPolicy: "allow", localPolicy: "allow" };

  const update = (patch: Partial<FirewallPolicy>) => mutation.mutate({ ...current, ...patch });

  return (
    <div className="grid gap-3 sm:grid-cols-2">
      <label className="flex items-start gap-2 rounded-lg border border-border/60 bg-muted/30 px-3 py-2.5 text-sm">
        <Globe className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
        <span className="flex-1">
          <span className="block font-medium">Para a internet (WAN)</span>
          <span className="block text-xs text-muted-foreground">Tráfego que nenhuma regra cobre.</span>
          <SelectNative
            className="mt-2"
            value={current.wanPolicy}
            disabled={policy.isLoading || mutation.isPending}
            onChange={(event) => update({ wanPolicy: event.target.value as FirewallPolicy["wanPolicy"] })}
          >
            <option value="allow">Permitir por padrão</option>
            <option value="deny">Bloquear por padrão</option>
          </SelectNative>
        </span>
      </label>
      <label className="flex items-start gap-2 rounded-lg border border-border/60 bg-muted/30 px-3 py-2.5 text-sm">
        <ServerCog className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
        <span className="flex-1">
          <span className="block font-medium">Para o painel/gateway (LOCAL)</span>
          <span className="block text-xs text-muted-foreground">DNS, DHCP e painel ficam sempre liberados.</span>
          <SelectNative
            className="mt-2"
            value={current.localPolicy}
            disabled={policy.isLoading || mutation.isPending}
            onChange={(event) => update({ localPolicy: event.target.value as FirewallPolicy["localPolicy"] })}
          >
            <option value="allow">Permitir por padrão</option>
            <option value="deny">Bloquear por padrão</option>
          </SelectNative>
        </span>
      </label>
    </div>
  );
}
