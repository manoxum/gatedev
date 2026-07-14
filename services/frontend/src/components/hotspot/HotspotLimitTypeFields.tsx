import type { FieldErrors, UseFormRegister } from "react-hook-form";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SelectNative } from "@/components/ui/select-native";
import { RateUnitOptions } from "@/components/hotspot/RateUnitOptions";
import { HotspotRateFields } from "@/components/hotspot/HotspotRateFields";
import { HotspotCreditConfigFields } from "@/components/hotspot/HotspotCreditConfigFields";
import { HotspotLimitTypeToggle } from "@/components/hotspot/HotspotLimitTypeToggle";
import type { LimitType } from "@/components/hotspot/hotspot-limits-types";

// Fieldset de limite de dispositivo (override) ou perfil - taxa
// (sempre independente do tipo) + seletor de tipo unico e mutuamente
// exclusivo (ilimitado/credito/cota) + bloco condicional pro tipo
// escolhido. Compartilhado por HotspotDeviceLimitsForm.tsx e
// HotspotProfileForm.tsx.
//
// O seletor de tipo (HotspotLimitTypeToggle) e um componente controlado
// (value/onChange), nao um <select> registrado via register() - o pai
// precisa passar limitType (via watch) e onLimitTypeChange (via
// setValue) em vez de so espalhar register nele.
//
// showCreditRechargeFields=false (uso em dispositivo): a politica de
// recarga de credito do dispositivo mora em endpoint/aba separados
// (DeviceCreditCard.tsx, PATCH .../credit) - aqui so o tipo e escolhido.
// Perfil funde tudo num formulario so, por isso mostra os campos de
// recarga inline (default true).
//
// includeCustom=true (uso em perfil, ver HotspotProfileForm.tsx): mostra
// a 4a opcao "Customizado" no seletor - quando escolhida, NENHUM campo
// deste fieldset (nem taxa) e mostrado, porque o perfil customizado nao
// aplica limite nenhum, o dispositivo que herda ele e quem define tudo.
export function HotspotLimitTypeFields({
  register,
  errors,
  limitType,
  onLimitTypeChange,
  showCreditRechargeFields = true,
  includeCustom = false,
}: {
  register: UseFormRegister<any>;
  errors: FieldErrors<any>;
  limitType: LimitType;
  onLimitTypeChange: (type: LimitType) => void;
  showCreditRechargeFields?: boolean;
  includeCustom?: boolean;
}) {
  return (
    <>
      {limitType !== "custom" && <HotspotRateFields register={register} errors={errors} />}

      <div className="space-y-2">
        <Label>Tipo de limitação</Label>
        <HotspotLimitTypeToggle value={limitType} onChange={onLimitTypeChange} includeCustom={includeCustom} />
      </div>

      {limitType === "custom" && (
        <p className="text-sm text-muted-foreground">
          Este perfil não aplica nenhum limite - o dispositivo que herdar este perfil deve definir a própria
          estratégia (tipo, taxa e cota/crédito) na aba "Limites" dele.
        </p>
      )}

      {limitType === "credit" &&
        (showCreditRechargeFields ? (
          <HotspotCreditConfigFields register={register} />
        ) : (
          <p className="text-sm text-muted-foreground">
            Saldo e recarga são configurados na aba "Crédito" do dispositivo.
          </p>
        ))}

      {limitType === "quota" && (
        <fieldset className="space-y-4">
          <legend className="text-sm font-medium text-muted-foreground">
            Cota de dados (defina uma, duas ou as três janelas)
          </legend>
          <div className="grid gap-4 sm:grid-cols-3">
            <div className="space-y-2">
              <Label htmlFor="dailyQuotaValue">Diária</Label>
              <div className="flex gap-2">
                <Input id="dailyQuotaValue" placeholder="sem teto" {...register("dailyQuotaValue")} />
                <SelectNative id="dailyQuotaUnit" className="w-20" {...register("dailyQuotaUnit")}>
                  <RateUnitOptions />
                </SelectNative>
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="weeklyQuotaValue">Semanal</Label>
              <div className="flex gap-2">
                <Input id="weeklyQuotaValue" placeholder="sem teto" {...register("weeklyQuotaValue")} />
                <SelectNative id="weeklyQuotaUnit" className="w-20" {...register("weeklyQuotaUnit")}>
                  <RateUnitOptions />
                </SelectNative>
              </div>
            </div>
            <div className="space-y-2">
              <Label htmlFor="monthlyQuotaValue">Mensal</Label>
              <div className="flex gap-2">
                <Input id="monthlyQuotaValue" placeholder="sem teto" {...register("monthlyQuotaValue")} />
                <SelectNative id="monthlyQuotaUnit" className="w-20" {...register("monthlyQuotaUnit")}>
                  <RateUnitOptions />
                </SelectNative>
              </div>
            </div>
          </div>
          <p className="text-xs text-muted-foreground">
            Estourar qualquer uma das janelas configuradas bloqueia o tráfego até o próximo período ou até um reset manual.
          </p>
        </fieldset>
      )}
    </>
  );
}
