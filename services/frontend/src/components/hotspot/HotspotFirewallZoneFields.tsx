import type { FieldErrors, UseFormRegister, UseFormSetValue } from "react-hook-form";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SelectNative } from "@/components/ui/select-native";
import type { HotspotCommRuleFormValues } from "@/components/hotspot/hotspot-isolation-schema";
import type { CommEndpointOption } from "@/components/hotspot/HotspotIsolationRuleDialog";

interface HotspotFirewallZoneFieldsProps {
  register: UseFormRegister<HotspotCommRuleFormValues>;
  setValue: UseFormSetValue<HotspotCommRuleFormValues>;
  errors: FieldErrors<HotspotCommRuleFormValues>;
  zone: "wan" | "local";
  sourceKind: string;
  profileOptions: CommEndpointOption[];
  deviceOptions: CommEndpointOption[];
}

// Campos das zonas wan (internet) e local (gateway): só a ORIGEM (quem,
// entre os clientes) + na wan um destino externo opcional. O destino é
// implícito (internet ou gateway) e o sentido é sempre cliente→destino.
export function HotspotFirewallZoneFields({
  register,
  setValue,
  errors,
  zone,
  sourceKind,
  profileOptions,
  deviceOptions,
}: HotspotFirewallZoneFieldsProps) {
  const options = sourceKind === "device" ? deviceOptions : profileOptions;
  return (
    <>
      <div className="space-y-2">
        <Label>Origem (quem)</Label>
        <SelectNative {...register("sourceKind", { onChange: () => setValue("sourceRef", "") })} aria-label="Tipo da origem">
          <option value="any">Todos os clientes</option>
          <option value="profile">Perfil</option>
          <option value="device">Cliente</option>
        </SelectNative>
        {sourceKind !== "any" && (
          <SelectNative {...register("sourceRef")} aria-label="Origem">
            <option value="">Escolha…</option>
            {options.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </SelectNative>
        )}
        {errors.sourceRef && <p className="text-sm text-destructive">{errors.sourceRef.message}</p>}
      </div>

      {zone === "wan" && (
        <div className="space-y-2">
          <Label htmlFor="dstHost">Destino externo (opcional)</Label>
          <Input id="dstHost" placeholder="ex.: 1.2.3.4 ou 10.0.0.0/24 — vazio = qualquer" {...register("dstHost")} />
          {errors.dstHost && <p className="text-sm text-destructive">{errors.dstHost.message}</p>}
        </div>
      )}
    </>
  );
}
