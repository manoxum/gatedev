import { useState } from "react";
import { Plus, TriangleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import { HotspotIsolationRuleDialog, type CommEndpointOption } from "@/components/hotspot/HotspotIsolationRuleDialog";
import { HotspotIsolationRulesTable } from "@/components/hotspot/HotspotIsolationRulesTable";
import { useHotspotCommRules, useHotspotIsolationState } from "@/components/hotspot/useHotspotIsolationQueries";
import { useHotspotIsolationMutations } from "@/components/hotspot/useHotspotIsolationMutations";
import { useHotspotProfiles } from "@/components/hotspot/useHotspotProfileQueries";
import type { HotspotCommRule, HotspotCommRuleRequest } from "@/components/hotspot/hotspot-isolation-types";
import type { HotspotKnownDevice } from "@/components/hotspot/useHotspotQueries";

interface HotspotIsolationCardProps {
  knownDevices: HotspotKnownDevice[];
}

// Aba Isolamento: interruptor geral (chave CLIENT_ISOLATION - só vale
// após reiniciar o hotspot) + regras de comunicação entre perfis e
// dispositivos. A precedência (mais específico vence, empate bloqueia)
// é avaliada no backend, ver hotspot_isolation_policy.go.
export function HotspotIsolationCard({ knownDevices }: HotspotIsolationCardProps) {
  const [ruleDialogOpen, setRuleDialogOpen] = useState(false);
  const [editingRule, setEditingRule] = useState<HotspotCommRule | null>(null);
  const [deletingRule, setDeletingRule] = useState<HotspotCommRule | null>(null);

  const isolation = useHotspotIsolationState();
  const rules = useHotspotCommRules();
  const profiles = useHotspotProfiles();
  const { setEnabled, createRule, updateRule, removeRule } = useHotspotIsolationMutations();

  const enabled = isolation.data?.enabled ?? false;

  const profileOptions: CommEndpointOption[] = (profiles.data ?? []).map((profile) => ({
    value: profile.id,
    label: profile.name,
  }));
  const deviceOptions: CommEndpointOption[] = knownDevices.map((device) => {
    const name = device.alias ?? device.deviceName ?? device.vendor;
    return { value: device.mac, label: name ? `${name} (${device.mac})` : device.mac };
  });

  // Nome legível de uma ponta (sem prefixo de tipo - a tabela adiciona
  // o contexto Perfil/Cliente na coluna correspondente).
  const endpointName = (kind: string, ref: string | null) => {
    if (kind === "any") return "Todos os clientes";
    if (kind === "profile") {
      return profileOptions.find((option) => option.value === ref)?.label ?? ref ?? "?";
    }
    return deviceOptions.find((option) => option.value === ref)?.label ?? ref ?? "?";
  };

  const submitRule = (request: HotspotCommRuleRequest) => {
    const onSuccess = () => {
      setRuleDialogOpen(false);
      setEditingRule(null);
    };
    if (editingRule) {
      updateRule.mutate({ id: editingRule.id, rule: request }, { onSuccess });
    } else {
      createRule.mutate(request, { onSuccess });
    }
  };

  const toggleRule = (rule: HotspotCommRule) => {
    updateRule.mutate({ id: rule.id, rule: { ...rule, enabled: !rule.enabled } });
  };

  return (
    <Card>
      <CardHeader className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <CardTitle>Isolamento de clientes</CardTitle>
        <Button
          size="sm"
          onClick={() => {
            setEditingRule(null);
            setRuleDialogOpen(true);
          }}
        >
          <Plus className="mr-1 h-4 w-4" />
          Nova regra
        </Button>
      </CardHeader>
      <CardContent className="space-y-4">
        <label className="flex items-start gap-2 rounded-lg border border-border/60 bg-muted/30 px-3 py-2.5 text-sm">
          <input
            type="checkbox"
            className="mt-0.5 h-4 w-4 accent-primary"
            checked={enabled}
            disabled={isolation.isLoading || setEnabled.isPending}
            onChange={(event) => setEnabled.mutate(event.target.checked)}
          />
          <span>
            <span className="font-medium">Ativar isolamento de clientes</span>
            <span className="block text-xs text-muted-foreground">
              Com o isolamento ativo, nenhum cliente comunica com outro por padrão — libere pelas regras abaixo.
              Tráfego para a internet e para o painel não é afetado.
            </span>
          </span>
        </label>

        <div className="flex items-start gap-2 rounded-lg border border-amber-500/40 bg-amber-500/10 px-3 py-2.5 text-xs text-muted-foreground">
          <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0 text-amber-500" />
          <span>
            Ligar/desligar o interruptor só vale de fato após <span className="font-medium">reiniciar o hotspot</span>{" "}
            (parar e iniciar no cartão acima) — as regras abaixo aplicam ao vivo, sem reiniciar. Com o isolamento
            ativo, descoberta por broadcast/mDNS (ex.: Chromecast, impressoras) não funciona entre clientes, mesmo
            entre pares permitidos.
          </span>
        </div>

        <HotspotIsolationRulesTable
          rules={rules.data ?? []}
          endpointName={endpointName}
          pendingId={updateRule.isPending ? updateRule.variables?.id : removeRule.isPending ? removeRule.variables : undefined}
          onEdit={(rule) => {
            setEditingRule(rule);
            setRuleDialogOpen(true);
          }}
          onToggleEnabled={toggleRule}
          onDelete={(rule) => setDeletingRule(rule)}
        />
      </CardContent>

      <HotspotIsolationRuleDialog
        open={ruleDialogOpen}
        onOpenChange={(open) => {
          setRuleDialogOpen(open);
          if (!open) setEditingRule(null);
        }}
        rule={editingRule}
        profileOptions={profileOptions}
        deviceOptions={deviceOptions}
        pending={createRule.isPending || updateRule.isPending}
        onSubmit={submitRule}
      />

      <ConfirmDialog
        open={!!deletingRule}
        onOpenChange={(open) => {
          if (!open) setDeletingRule(null);
        }}
        title="Remover regra de comunicação?"
        description="Os clientes afetados voltam a seguir só a comunicação interna dos perfis e as demais regras."
        confirmLabel="Remover"
        variant="destructive"
        pending={removeRule.isPending}
        onConfirm={() => {
          if (!deletingRule) return;
          removeRule.mutate(deletingRule.id, { onSuccess: () => setDeletingRule(null) });
        }}
      />
    </Card>
  );
}
