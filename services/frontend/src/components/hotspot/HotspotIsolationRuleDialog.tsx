import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SelectNative } from "@/components/ui/select-native";
import { HotspotRuleScopeToggle } from "@/components/hotspot/HotspotRuleScopeToggle";
import { HotspotIsolationEndpointsFields } from "@/components/hotspot/HotspotIsolationEndpointsFields";
import { HotspotFirewallZoneToggle } from "@/components/hotspot/HotspotFirewallZoneToggle";
import { HotspotFirewallZoneFields } from "@/components/hotspot/HotspotFirewallZoneFields";
import {
  hotspotCommRuleFormSchema,
  commRuleToFormValues,
  emptyCommRuleFormValues,
  formValuesToCommRule,
  type HotspotCommRuleFormValues,
} from "@/components/hotspot/hotspot-isolation-schema";
import type { HotspotCommRule, HotspotCommRuleRequest } from "@/components/hotspot/hotspot-isolation-types";

export interface CommEndpointOption {
  value: string;
  label: string;
}

interface HotspotIsolationRuleDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  // null = criar regra nova; caso contrário edita a existente.
  rule: HotspotCommRule | null;
  profileOptions: CommEndpointOption[];
  deviceOptions: CommEndpointOption[];
  pending: boolean;
  onSubmit: (rule: HotspotCommRuleRequest) => void;
}

// Formulário de regra com duas modalidades ("escopo"): comunicação
// dentro de um perfil (uma escolha só) ou entre uma origem e um destino
// distintos. O sentido "destino → origem" da modalidade origem/destino
// é gravado com as pontas trocadas (o backend só conhece "to"/"both",
// ver hotspot-isolation-schema.ts).
export function HotspotIsolationRuleDialog({
  open,
  onOpenChange,
  rule,
  profileOptions,
  deviceOptions,
  pending,
  onSubmit,
}: HotspotIsolationRuleDialogProps) {
  const {
    register,
    handleSubmit,
    watch,
    setValue,
    formState: { errors },
  } = useForm<HotspotCommRuleFormValues>({
    resolver: zodResolver(hotspotCommRuleFormSchema),
    values: rule ? commRuleToFormValues(rule) : emptyCommRuleFormValues,
  });
  const zone = watch("zone");
  const scope = watch("scope");
  const sourceKind = watch("sourceKind");
  const targetKind = watch("targetKind");
  const protocol = watch("protocol");
  const portsEnabled = protocol === "tcp" || protocol === "udp";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{rule ? "Editar regra de comunicação" : "Nova regra de comunicação"}</DialogTitle>
          <DialogDescription>
            Regras mais específicas vencem (cliente &gt; perfil &gt; todos); em empate, bloquear vence permitir.
          </DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={handleSubmit((values) => onSubmit(formValuesToCommRule(values)))}>
          <div className="space-y-2">
            <Label>Zona da regra</Label>
            <HotspotFirewallZoneToggle value={zone} onChange={(value) => setValue("zone", value, { shouldValidate: true })} />
          </div>

          {zone !== "clients" ? (
            <HotspotFirewallZoneFields
              register={register}
              setValue={setValue}
              errors={errors}
              zone={zone}
              sourceKind={sourceKind}
              profileOptions={profileOptions}
              deviceOptions={deviceOptions}
            />
          ) : scope === "within-profile" ? (
            <>
              <div className="space-y-2">
                <Label>O que esta regra controla?</Label>
                <HotspotRuleScopeToggle value={scope} onChange={(value) => setValue("scope", value, { shouldValidate: true })} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="profileRef">Perfil</Label>
                <SelectNative id="profileRef" {...register("profileRef")}>
                  <option value="">Escolha…</option>
                  {profileOptions.map((option) => (
                    <option key={option.value} value={option.value}>
                      {option.label}
                    </option>
                  ))}
                </SelectNative>
                <p className="text-xs text-muted-foreground">
                  {watch("action") === "allow" ? "Permite" : "Bloqueia"} a comunicação entre os clientes deste perfil.
                </p>
                {errors.profileRef && <p className="text-sm text-destructive">{errors.profileRef.message}</p>}
              </div>
            </>
          ) : (
            <>
              <div className="space-y-2">
                <Label>O que esta regra controla?</Label>
                <HotspotRuleScopeToggle value={scope} onChange={(value) => setValue("scope", value, { shouldValidate: true })} />
              </div>
              <HotspotIsolationEndpointsFields
                register={register}
                setValue={setValue}
                errors={errors}
                sourceKind={sourceKind}
                targetKind={targetKind}
                profileOptions={profileOptions}
                deviceOptions={deviceOptions}
              />
            </>
          )}

          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="protocol">Protocolo</Label>
              <SelectNative
                id="protocol"
                {...register("protocol", {
                  onChange: (event) => {
                    if (event.target.value !== "tcp" && event.target.value !== "udp") setValue("dstPorts", "");
                  },
                })}
              >
                <option value="any">Qualquer</option>
                <option value="tcp">TCP</option>
                <option value="udp">UDP</option>
                <option value="icmp">ICMP (ping)</option>
              </SelectNative>
            </div>
            <div className="space-y-2">
              <Label htmlFor="dstPorts">Portas de destino</Label>
              <Input
                id="dstPorts"
                placeholder={portsEnabled ? "ex.: 80,443,8000-8100" : "só para TCP/UDP"}
                disabled={!portsEnabled}
                {...register("dstPorts")}
              />
              {errors.dstPorts && <p className="text-sm text-destructive">{errors.dstPorts.message}</p>}
            </div>
          </div>

          <div className="grid gap-4 sm:grid-cols-2">
            <div className="space-y-2">
              <Label htmlFor="action">Ação</Label>
              <SelectNative id="action" {...register("action")}>
                <option value="allow">Permitir</option>
                <option value="deny">Bloquear</option>
              </SelectNative>
            </div>
            <div className="space-y-2">
              <Label htmlFor="note">Observação (opcional)</Label>
              <Input id="note" placeholder="ex.: impressora dos convidados" {...register("note")} />
            </div>
          </div>

          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" className="h-4 w-4 accent-primary" {...register("enabled")} />
            Regra ativa
          </label>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={pending}>
              Cancelar
            </Button>
            <Button type="submit" disabled={pending}>
              {rule ? "Salvar" : "Criar regra"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
