import type { UseFormRegister } from "react-hook-form";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SelectNative } from "@/components/ui/select-native";

// Fieldset de politica de recarga de credito compartilhado por
// DeviceCreditCard (config por dispositivo) e HotspotLimitTypeFields
// (perfil/override, via limitType="credit") - os dois usam os mesmos
// nomes de campo (rechargeAmountGB/rechargePeriod/plafondGB), so muda o
// que o formulario pai faz no submit. Sem checkbox "enabled" - quem
// decide se essa politica esta ativa e o limitType do pai (o proprio
// fato deste fieldset estar renderizado ja significa que sim), nunca um
// campo independente aqui. register fica frouxamente tipado
// (UseFormRegister<any>) de proposito, ja que este fieldset e reusado
// por schemas zod diferentes que so compartilham esse subconjunto de
// campos.
export function HotspotCreditConfigFields({
  register,
  idPrefix = "",
}: {
  register: UseFormRegister<any>;
  idPrefix?: string;
}) {
  return (
    <fieldset className="space-y-4">
      <legend className="text-sm font-medium text-muted-foreground">Recarga automática (opcional)</legend>
      <div className="grid gap-4 sm:grid-cols-3">
        <div className="space-y-2">
          <Label htmlFor={`${idPrefix}rechargeAmountGB`}>Recarga por período (GB)</Label>
          <Input id={`${idPrefix}rechargeAmountGB`} placeholder="só manual" {...register("rechargeAmountGB")} />
        </div>
        <div className="space-y-2">
          <Label htmlFor={`${idPrefix}rechargePeriod`}>Período</Label>
          <SelectNative id={`${idPrefix}rechargePeriod`} {...register("rechargePeriod")}>
            <option value="daily">Diário</option>
            <option value="weekly">Semanal</option>
            <option value="monthly">Mensal</option>
          </SelectNative>
        </div>
        <div className="space-y-2">
          <Label htmlFor={`${idPrefix}plafondGB`}>Plafond - teto do saldo (GB)</Label>
          <Input id={`${idPrefix}plafondGB`} placeholder="sem teto" {...register("plafondGB")} />
        </div>
      </div>
    </fieldset>
  );
}
